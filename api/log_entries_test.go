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

package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/transparency-dev/armored-witness-applet/api"
)

func TestParseFirmwareRelease(t *testing.T) {
	bs, err := os.ReadFile("example_firmware_release.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	r := api.FirmwareRelease{}
	if err := json.Unmarshal(bs, &r); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got, want := r.Component, api.ComponentApplet; got != want {
		t.Errorf("Got %q, want %q", got, want)
	}
	if got, want := r.GitTagName, "0.1.2"; got != want {
		t.Errorf("Got %q, want %q", got, want)
	}
	if got, want := len(r.GitCommitFingerprint), 40; got != want {
		t.Errorf("Got %d, want %d", got, want)
	}
	if got, want := r.GitCommitFingerprint, "aac1e176cfac1a1e079b5f624b83fda54b5d0f76"; got != want {
		t.Errorf("Got %x, want %x", got, want)
	}
	if got, want := len(r.FirmwareDigestSha256), 32; got != want {
		t.Errorf("Got %d, want %d", got, want)
	}
	if got, want := r.FirmwareDigestSha256, mustDecode("8l4TaroPsSq+zwG+XMPZw+EdpUoXH0IT4cKM2RmFyNE="); !bytes.Equal(got, want) {
		t.Errorf("Got %x, want %x", got, want)
	}
	if got, want := r.TamagoVersion, "1.20.6"; got != want {
		t.Errorf("Got %q, want %q", got, want)
	}
}

func mustDecode(in string) []byte {
	r, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}
	return r
}
