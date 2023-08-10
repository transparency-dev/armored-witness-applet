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

package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/coreos/go-semver/semver"
	"github.com/transparency-dev/armored-witness-applet/api"
)

func main() {
	gitTag := flag.String("git_tag", "",
		"The semantic version of the Trusted Applet release.")
	gitCommitFingerprint := flag.String("git_commit_fingerprint", "",
		"Hex-encoded SHA-1 commit hash of the git repository when checked out at the specified git_tag.")
	firmwareFingerprint := flag.String("firmware_fingerprint", "",
		"Hex-encoded hash of the compiled firmware binary. ")
	tamagoVersion := flag.String("tamago_version", "",
		"The version of the Tamago (https://github.com/usbarmory/tamago) used to compile the Trusted Applet.")

	flag.Parse()

	if *gitTag == "" {
		log.Fatal("git_tag is required.")
	}
	if *gitCommitFingerprint == "" {
		log.Fatal("git_commit_fingerprint is required.")
	}
	if *firmwareFingerprint == "" {
		log.Fatal("firmware_fingerprint is required.")
	}
	if *tamagoVersion == "" {
		log.Fatal("tamago_version is required.")
	}

	digestBytes, err := hex.DecodeString(*firmwareFingerprint)
	if err != nil {
		log.Fatalf("Failed to hex-deocode string %q: %v", *firmwareFingerprint, err)
	}

	r := api.FirmwareRelease{
		Component:            api.ComponentApplet,
		GitTagName:           *semver.New(*gitTag),
		GitCommitFingerprint: *gitCommitFingerprint,
		FirmwareDigestSha256: digestBytes,
		TamagoVersion:        *semver.New(*tamagoVersion),
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(b))
	return
}
