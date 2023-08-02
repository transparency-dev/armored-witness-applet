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

// Package api contains public structures related to the log contents.
package api

const (
	// Component name for the applet used in FirmwareRelease.Component.
	ComponentApplet = "TRUSTED_APPLET"
)

// FirmwareRelease represents a firmware release in the log.
type FirmwareRelease struct {
	// Component identifies the type of firmware (e.g. OS or applet).
	// This component is key to disambiguate what the firmware is, and other
	// implicit information can be derived from this. For example, the git
	// repository that the code should be checked out from to reproduce the
	// build.
	Component string `json:"component"`

	// TagName identifies the version of this release, e.g. "0.1.2"
	TagName string `json:"tag_name"`

	// CommitHash contains the SHA256 commit hash of the git repository when checked
	// out at TagName. Committing to this information allows verifiers that cannot
	// reproduce a build to quickly narrow down the problem space:
	//  - if this CommitHash is different then they have checked out different code
	//    than was used to build the binary. This could happen if the wrong repo was
	//    used, or because the TagName was changed to a different commit
	//  - if the CommitHash is the same, then they have the same code checked out but
	//    there is a problem with the build toolchain (different tooling or non-reproducible
	//    builds).
	CommitHash []byte `json:"commit_sha"`

	// Sha256Digest is the hash of the compiled firmware binary. Believers that are
	// installing a firmware release must check that the firmware data they are going to
	// believe has a fingerprint matching this hash. Verifiers that check out the correct
	// source repo & version must be able to reproducibly build a binary that has this fingerprint.
	Sha256Digest []byte `json:"sha256_digest"`

	// TamagoVersion identifies the version of [Tamago] that the builder used to compile
	// the binary with Sha256Digest.
	//
	// [Tamago]: https://github.com/usbarmory/tamago
	TamagoVersion string `json:"tamago_version"`
}
