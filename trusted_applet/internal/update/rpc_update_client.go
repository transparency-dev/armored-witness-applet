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

// Package update provides functionality for fetching updates, verifying
// them, and installing them onto the armory device.
package update

import (
	"github.com/coreos/go-semver/semver"
	"github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/firmware"
	"github.com/transparency-dev/armored-witness-boot/config"
	"github.com/transparency-dev/armored-witness-os/api/rpc"
	"github.com/usbarmory/GoTEE/syscall"
)

// RPCClient is an implementation of the Local interface which uses RPCs to the TrustedOS
// to perform the updates.
type RPCClient struct {
}

// GetInstalledVersions returns the semantic versions of the OS and Applet
// installed on this device. These will be the same versions that are
// currently running.
func (r *RPCClient) GetInstalledVersions() (os, applet semver.Version, err error) {
	iv := &rpc.InstalledVersions{}
	err = syscall.Call("RPC.GetInstalledVersions", nil, iv)
	return iv.OS, iv.Applet, err

}

// InstallOS updates the OS to the version contained in the firmware bundle.
func (r *RPCClient) InstallOS(fb firmware.Bundle) error {
	fu := &rpc.FirmwareUpdate{
		Image: fb.Firmware,
		Proof: config.ProofBundle{
			Checkpoint:     fb.Checkpoint,
			LogIndex:       fb.Index,
			InclusionProof: fb.InclusionProof,
			Manifest:       fb.Manifest,
		},
	}
	err := syscall.Call("RPC.InstallOS", nil, fu)
	return err

}

// InstallApplet updates the Applet to the version contained in the firmware bundle.
func (r *RPCClient) InstallApplet(fb firmware.Bundle) error {
	fu := &rpc.FirmwareUpdate{
		Image: fb.Firmware,
		Proof: config.ProofBundle{
			Checkpoint:     fb.Checkpoint,
			LogIndex:       fb.Index,
			InclusionProof: fb.InclusionProof,
			Manifest:       fb.Manifest,
		},
	}
	err := syscall.Call("RPC.InstallApplet", nil, fu)
	return err
}

// Reboot instructs the device to reboot after new firmware is installed.
// This call will not return and deferred functions will not be run.
func (r *RPCUpdate) Reboot() {
	_ = syscall.Call("RPC.Reboot", nil, nil)
}
