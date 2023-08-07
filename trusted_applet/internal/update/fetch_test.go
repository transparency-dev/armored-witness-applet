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
	"fmt"
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/transparency-dev/armored-witness-applet/api"
	"github.com/transparency-dev/formats/log"
	"golang.org/x/mod/sumdb/note"
)

const (
	vkey = "ArmoredWitnessFirmwareLog+3e6f9306+ARjETaImkiqXZCH5pk1XtfX0tHgFhi1qGIxQqT6231S1"
	skey = "PRIVATE+KEY+ArmoredWitnessFirmwareLog+3e6f9306+AYJIjPyyT5wKmBQ8duU8Bwl2ZSslUmrMgwdTUChHKEag"
)

func TestFetcher(t *testing.T) {
	logClient := &fakeLogClient{
		releases: []api.FirmwareRelease{
			{
				Component:  api.ComponentOS,
				GitTagName: *semver.New("1.0.1"),
			},
			{
				Component:  api.ComponentApplet,
				GitTagName: *semver.New("1.1.1"),
			},
		},
	}
	f := NewHttpFetcher(logClient, vkey)

	f.Scan()

	os, applet, err := f.GetLatestVersions()
	if err != nil {
		t.Fatalf("GetLatestVersions(): %v", err)
	}
	if got, want := os, *semver.New("1.0.1"); got != want {
		t.Errorf("got != want (%v, %v)", got, want)
	}
	if got, want := applet, *semver.New("1.1.1"); got != want {
		t.Errorf("got != want (%v, %v)", got, want)
	}

	logClient.releases = append(logClient.releases, []api.FirmwareRelease{
		{
			Component:  api.ComponentOS,
			GitTagName: *semver.New("1.2.1"),
		},
		{
			Component:  api.ComponentApplet,
			GitTagName: *semver.New("1.3.1"),
		},
	}...)

	f.Scan()
	os, applet, err = f.GetLatestVersions()
	if err != nil {
		t.Fatalf("GetLatestVersions(): %v", err)
	}
	if got, want := os, *semver.New("1.2.1"); got != want {
		t.Errorf("got != want (%v, %v)", got, want)
	}
	if got, want := applet, *semver.New("1.3.1"); got != want {
		t.Errorf("got != want (%v, %v)", got, want)
	}
}

type fakeLogClient struct {
	releases []api.FirmwareRelease
}

func (c *fakeLogClient) GetLeafAndInclusion(index, treeSize uint64) ([]byte, [][]byte, error) {
	if treeSize > uint64(len(c.releases)) {
		return nil, nil, fmt.Errorf("treeSize %d out of bounds for log with %d entries", treeSize, len(c.releases))
	}
	manifest := c.releases[index]
	bs, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}
	// TODO(mhutchinson): inclusion proofs
	return bs, nil, nil
}

func (c *fakeLogClient) GetBinary(release api.FirmwareRelease) ([]byte, error) {
	return []byte(release.GitTagName.String()), nil
}

func (c *fakeLogClient) GetLatestCheckpoint() ([]byte, error) {
	cp := log.Checkpoint{
		Origin: origin,
		Size:   uint64(len(c.releases)),
	}
	n := note.Note{
		Text: string(cp.Marshal()),
	}
	signer, err := note.NewSigner(skey)
	if err != nil {
		return nil, err
	}
	return note.Sign(&n, signer)
}
