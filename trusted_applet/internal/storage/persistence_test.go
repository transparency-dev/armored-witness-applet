// Copyright 2025 The Armored Witness Applet authors. All Rights Reserved.
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
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUnmarshalCheckpoint(t *testing.T) {
	for _, test := range []struct {
		name    string
		cpRaw   []byte
		wantCP  []byte
		wantErr bool
	}{
		{
			name:   "YAML to CP",
			cpRaw:  marshalOldCheckpoint(t, []byte("CP")),
			wantCP: []byte("CP"),
		}, {
			name:   "RAW to CP",
			cpRaw:  marshalCheckpoint([]byte("CP")),
			wantCP: []byte("CP"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gotCP, gotErr := unmarshalCheckpoint(test.cpRaw)
			if e := gotErr != nil; e != test.wantErr {
				t.Fatalf("Got error %q want err: %t", gotErr, test.wantErr)
			}
			if !bytes.Equal(gotCP, test.wantCP) {
				t.Fatalf("Got CP %q want %q", gotCP, test.wantCP)
			}
		})
	}
}

func TestCheckpointDoubleRoundtrip(t *testing.T) {
	want := []byte("A Pwnie")
	oldFmt := marshalOldCheckpoint(t, want)
	got1, err := unmarshalCheckpoint(oldFmt)
	if err != nil {
		t.Fatalf("Failed to unmarshal old format checkpoint: %v", err)
	}
	if !bytes.Equal(got1, want) {
		t.Fatalf("Got %q from old format, want %q", got1, want)
	}
	rawFmt := marshalCheckpoint(got1)
	if bytes.Equal(oldFmt, rawFmt) {
		t.Errorf("Expected different encodings; old %q new %q", oldFmt, rawFmt)
	}
	got2, err := unmarshalCheckpoint(rawFmt)
	if err != nil {
		t.Fatalf("Failed to unmarshal raw format checkpoint: %v", err)
	}
	if !bytes.Equal(got2, want) {
		t.Fatalf("Got %q from raw format, want %q", got1, want)
	}
}

func marshalOldCheckpoint(t *testing.T, cp []byte) []byte {
	t.Helper()
	y, err := yaml.Marshal(struct {
		Checkpoint []byte
		Proof      []byte
	}{
		Checkpoint: cp,
	})
	if err != nil {
		t.Fatalf("Failed to marshal yaml: %v", err)
	}
	return y
}
