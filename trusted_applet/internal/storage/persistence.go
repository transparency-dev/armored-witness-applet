// Copyright 2022 The Armored Witness Applet authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/storage/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

const (
	mappingConfigSlot = 0
)

var (
	// rawRecordMagic is a prefix which denotes that the bytes following it are stored "raw",
	// as opposed to in a YAML serialised logRecord struct.
	// The magic string contains a "control" character which the YAML spec explicitly forbids,
	// protecting against misinterpretation.
	rawRecordMagic = []byte("\x01RAW")
)

// SlotPersistence is an implementation of the witness Persistence
// interface based on Slots.
type SlotPersistence struct {
	// mu protects access to everything below.
	mu sync.RWMutex

	// part is the underlying storage partition we're using to persist
	// data.
	part *slots.Partition

	// directorySlot is a reference to the zeroth slot in a partition.
	// This slot is used to maintain a mapping of log ID to slot index
	// where state for that log is stored.
	directorySlot *slots.Slot
	// directoryWriteToken is the token received when we read the mapping config
	// from the mapSlot above. It'll be used when we want to store an updated
	// mapping config.
	directoryWriteToken uint32

	// idToSlot maintains the mapping from LogID to slot index used to store
	// checkpoints from that log.
	idToSlot slotMap

	// freeSlots is a list of unused slot indices available to be mapped to logIDs.
	freeSlots []uint
}

// slotMap defines the structure of the mapping config stored in slot zero.
type slotMap map[string]uint

// NewSlotPersistence creates a new SlotPersistence instance.
// As per the Persistence interface, Init must be called before it's used to
// read or write any data.
func NewSlotPersistence(part *slots.Partition) *SlotPersistence {
	return &SlotPersistence{
		part:     part,
		idToSlot: make(map[string]uint),
	}
}

// Init sets up the persistence layer. This should be idempotent,
// and will be called once per process startup.
func (p *SlotPersistence) Init(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, err := p.part.Open(mappingConfigSlot)
	if err != nil {
		return fmt.Errorf("failed to open mapping slot: %v", err)
	}
	p.directorySlot = s
	if err := p.populateMap(); err != nil {
		return fmt.Errorf("failed to populate logID → slot map: %v", err)
	}
	return nil
}

// marshalCheckpoint knows how to serialise a checkpoint for storage by the
// persistence.
func marshalCheckpoint(cpRaw []byte) []byte {
	// Return a new slice which contains the magic raw prefix followed directly by the checkpoint bytes.
	return append(append(make([]byte, 0, len(rawRecordMagic)+len(cpRaw)), rawRecordMagic...), cpRaw...)
}

// unmarchalCheckpoint knows how to unmarshal a checkpoint which has been
// stored by the persistence.
//
// Think very carefully before removing this method - there _could_ be a checkpoint which
// was stored back when the YAML encoding was used, and then never updated (e.g. because
// the log never grew), until suddenly it _did_ grow.
func unmarshalCheckpoint(b []byte) ([]byte, error) {
	// Optimistically check whether this record has been updated to not use YAML,
	// and simply return the bytes as-is if so:
	if b, ok := bytes.CutPrefix(b, rawRecordMagic); ok {
		return b, nil
	}
	// Othersize fall-back to reading legacy encoding.
	lr := struct {
		Checkpoint []byte
		Proof      []byte
	}{}
	if err := yaml.Unmarshal(b, &lr); err != nil {
		klog.Warningf("Unmarshal failed: %v", err)
		return nil, fmt.Errorf("failed to unmarshal data: %v", err)
	}
	return lr.Checkpoint, nil
}

// Latest returns the last recorded checkpoint for the given logID, or an
// `codes.NotFound` error if no such checkpoint has been recorded.
//
// Implements the omniwitness LogPersistence interface.
func (p *SlotPersistence) Latest(_ context.Context, logID string) ([]byte, error) {
	i, err := p.logSlot(logID, false)
	if err != nil {
		return nil, err
	}
	s, err := p.part.Open(i)
	if err != nil {
		return nil, fmt.Errorf("internal error opening slot %d associated with log ID %q: %v", i, logID, err)
	}
	b, _, err := s.Read()
	if err != nil {
		klog.Warningf("Read failed: %v", err)
		return nil, fmt.Errorf("failed to read data: %v", err)
	}
	if len(b) == 0 {
		return nil, status.Error(codes.NotFound, "no checkpoint for log")
	}
	return unmarshalCheckpoint(b)
}

// Update allows for storing a new checkpoint for a given LogID.
//
// Implements the omniwitness LogPersistence interface.
func (p *SlotPersistence) Update(_ context.Context, logID string, f func(current []byte) ([]byte, error)) error {
	i, err := p.logSlot(logID, true)
	if err != nil {
		return err
	}
	s, err := p.part.Open(i)
	if err != nil {
		return fmt.Errorf("internal error opening slot %d associated with log ID %q: %v", i, logID, err)
	}
	b, t, err := s.Read()
	if err != nil {
		klog.Warningf("Read failed: %v", err)
		return fmt.Errorf("failed to read data: %v", err)
	}

	currCP, err := unmarshalCheckpoint(b)
	if err != nil {
		return fmt.Errorf("unmarshalCheckpoint: %v", err)
	}

	newCP, err := f(currCP)
	if err != nil {
		return err
	}
	if err := s.CheckAndWrite(t, marshalCheckpoint(newCP)); err != nil {
		klog.Warningf("Write failed: %v", err)
		return fmt.Errorf("failed to write data: %v", err)
	}
	return nil
}

// logSlot looks up the slot assigned to a given logID, optionally creating a mapping if there isn't
// a slot currently assigned.
func (p *SlotPersistence) logSlot(logID string, create bool) (uint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i, ok := p.idToSlot[logID]
	if !ok {
		if !create {
			return 0, status.Error(codes.NotFound, "no slot for log")
		}
		var err error
		i, err = p.addLog(logID)
		if err != nil {
			klog.Warningf("Failed to add mapping: %q", err)
			return 0, fmt.Errorf("unable to assign slot for log ID %q: %v", logID, err)
		}
		klog.V(2).Infof("Added mapping %q -> %d", logID, i)
	}
	return i, nil
}

// populateMap reads the logID -> slot mapping from storage.
// Must be called with p.mu write-locked.
func (p *SlotPersistence) populateMap() error {
	b, t, err := p.directorySlot.Read()
	if err != nil {
		return fmt.Errorf("failed to read persistence mapping: %v", err)
	}
	if err := yaml.Unmarshal(b, &p.idToSlot); err != nil {
		return fmt.Errorf("failed to unmarshal persistence mapping: %v", err)
	}
	// We read the logID<->Slot config, so save the token for if/when we want to
	// store an updated mapping.
	p.directoryWriteToken = t

	// Precalculate the list of available slots.
	slotState := make([]bool, p.part.NumSlots())
	for _, idx := range p.idToSlot {
		if idx == mappingConfigSlot {
			return errors.New("internal-error, reserved slot 0 has been used")
		}
		slotState[idx] = true
	}

	// Slot 0 is reserved for the mapping config, so mark it used here:
	slotState[mappingConfigSlot] = true

	p.freeSlots = make([]uint, 0, p.part.NumSlots())
	for idx, used := range slotState {
		if !used {
			p.freeSlots = append(p.freeSlots, uint(idx))
		}
	}
	return nil
}

// storeDirectory writes the current logID -> slot map to storage.
// Must be called with p.mu at leaest read-locked.
func (p *SlotPersistence) storeDirectory() error {
	smRaw, err := yaml.Marshal(p.idToSlot)
	if err != nil {
		return fmt.Errorf("failed to marshal mapping: %v", err)
	}
	if err := p.directorySlot.CheckAndWrite(p.directoryWriteToken, smRaw); err != nil {
		return fmt.Errorf("failed to store mapping: %v", err)
	}
	// TODO(al): CheckAndWrite should return the next token rather than us knowing
	// how the token changes after a successful write.
	p.directoryWriteToken++
	return nil
}

// addLog assigns a slot to a new log ID.
// Must be called with p.mu write-locked.
func (p *SlotPersistence) addLog(id string) (uint, error) {
	if idx, ok := p.idToSlot[id]; ok {
		return idx, nil
	}
	if len(p.freeSlots) == 0 {
		return 0, errors.New("no free slot available")
	}
	f := p.freeSlots[0]
	p.freeSlots = p.freeSlots[1:]
	p.idToSlot[id] = f
	if err := p.storeDirectory(); err != nil {
		return 0, fmt.Errorf("failed to storeMap: %v", err)
	}
	klog.V(1).Infof("Added new mapping %q -> %d", id, f)
	return f, nil
}
