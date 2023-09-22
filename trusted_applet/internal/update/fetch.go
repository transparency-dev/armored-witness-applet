// Copyright 2023 The Armored Witness Applet authors. All Rights Reserved.
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

package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/coreos/go-semver/semver"
	"github.com/transparency-dev/armored-witness-applet/api"
	"github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/firmware"
	"github.com/transparency-dev/formats/log"
	"golang.org/x/mod/sumdb/note"
	"k8s.io/klog/v2"
)

const (
	// TODO(mhutchinson): this should be defined outside of this file.
	origin = "transparency.dev/armored-witness/firmware_transparency/prod/0"
)

// LogClient fetches data from the log.
type LogClient interface {
	// GetLeafAndInclusion returns a raw leaf preimage and an inclusion proof for the given
	// index in the specified tree size.
	GetLeafAndInclusion(index, treeSize uint64) ([]byte, [][]byte, error)
	// GetBinary returns the firmware image corresponding to the given release.
	GetBinary(release api.FirmwareRelease) ([]byte, error)
	// GetLatestCheckpoint returns the largest checkpoint the log has available.
	GetLatestCheckpoint() ([]byte, error)
}

// NewHttpFetcher returns an implementation of a Remote that uses the given log client to
// fetch release data from the log.
func NewHttpFetcher(client LogClient, vkey string) *HttpFetcher {
	v, err := note.NewVerifier(vkey)
	if err != nil {
		panic(err)
	}
	f := &HttpFetcher{
		client:      client,
		logVerifier: v,
	}
	return f
}

type HttpFetcher struct {
	client      LogClient
	logVerifier note.Verifier

	mu           sync.Mutex
	latest       log.Checkpoint
	latestOS     *firmwareRelease
	latestApplet *firmwareRelease
}

func (f *HttpFetcher) GetLatestVersions() (os semver.Version, applet semver.Version, err error) {
	if f.latestOS == nil || f.latestApplet == nil {
		return semver.Version{}, semver.Version{}, errors.New("no versions of OS or applet found in log")
	}
	return f.latestOS.manifest.GitTagName, f.latestApplet.manifest.GitTagName, nil
}

func (f *HttpFetcher) GetOS() (firmware.Bundle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.latestOS == nil {
		return firmware.Bundle{}, errors.New("no latest OS available")
	}
	if f.latestOS.bundle.Firmware == nil {
		binary, err := f.client.GetBinary(f.latestOS.manifest)
		if err != nil {
			return firmware.Bundle{}, fmt.Errorf("GetBinary(): %v", err)
		}
		f.latestOS.bundle.Firmware = binary
	}
	return *f.latestOS.bundle, nil
}

func (f *HttpFetcher) GetApplet() (firmware.Bundle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.latestApplet == nil {
		return firmware.Bundle{}, errors.New("no latest applet available")
	}
	if f.latestApplet.bundle.Firmware == nil {
		binary, err := f.client.GetBinary(f.latestApplet.manifest)
		if err != nil {
			return firmware.Bundle{}, fmt.Errorf("GetBinary(): %v", err)
		}
		f.latestApplet.bundle.Firmware = binary
	}
	return *f.latestApplet.bundle, nil
}

// Scan gets the latest checkpoint from the log and updates the fetcher's state
// to reflect the latest OS and Applet available in the log.
func (f *HttpFetcher) Scan() error {
	cpRaw, err := f.client.GetLatestCheckpoint()
	if err != nil {
		return fmt.Errorf("GetLatestCheckpoint(): %v", err)
	}
	to, _, _, err := log.ParseCheckpoint(cpRaw, origin, f.logVerifier)
	if err != nil {
		return fmt.Errorf("ParseCheckpoint(): %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	from := f.latest.Size
	if to.Size <= from {
		return nil
	}
	for i := from; i < to.Size; i++ {
		leaf, inc, err := f.client.GetLeafAndInclusion(i, to.Size)
		if err != nil {
			return fmt.Errorf("failed to get log leaf %d: %v", i, err)
		}
		manifest, err := parseLeaf(leaf)
		if err != nil {
			return fmt.Errorf("failed to parse leaf at %d: %v", i, err)
		}
		bundle := &firmware.Bundle{
			Checkpoint:     cpRaw,
			Index:          i,
			InclusionProof: inc,
			Manifest:       leaf,
			Firmware:       nil, // This will be downloaded on demand
		}
		switch manifest.Component {
		case api.ComponentOS:
			if f.latestOS == nil || f.latestOS.manifest.GitTagName.LessThan(manifest.GitTagName) {
				f.latestOS = &firmwareRelease{
					bundle:   bundle,
					manifest: manifest,
				}
			}
		case api.ComponentApplet:
			if f.latestApplet == nil || f.latestApplet.manifest.GitTagName.LessThan(manifest.GitTagName) {
				f.latestApplet = &firmwareRelease{
					bundle:   bundle,
					manifest: manifest,
				}
			}
		default:
			klog.Warningf("unknown build in log: %q", manifest.Component)
		}
	}
	f.latest = *to
	return nil
}

func parseLeaf(leaf []byte) (api.FirmwareRelease, error) {
	r := api.FirmwareRelease{}
	if err := json.Unmarshal(leaf, &r); err != nil {
		return r, fmt.Errorf("Unmarshal: %v", err)
	}
	return r, nil
}

type firmwareRelease struct {
	bundle   *firmware.Bundle
	manifest api.FirmwareRelease
}
