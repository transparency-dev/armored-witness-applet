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
	"crypto/aes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"log"

	"github.com/goombaio/namegenerator"
	"github.com/transparency-dev/armored-witness-os/api"
	"github.com/usbarmory/GoTEE/syscall"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/mod/sumdb/note"
)

var (
	witnessPublicKey  string
	witnessSigningKey string
)

// deriveWitnessKey creates this witness' signing identity by deriving a key
// based on the hardware's unique internal secret key.
//
// Since we never store this derived key anywhere, for any given device this
// function MUST reproduce the same key on each boot (until the device is wiped,
// at which point a new stable key should be returned).
func deriveWitnessKey() {
	var status api.Status
	if err := syscall.Call("RPC.Status", nil, &status); err != nil {
		log.Fatalf("Failed to fetch Status: %v", err)
	}

	// We should add an obvious prefix to key names when we're running without secure boot
	prefix := ""
	if !status.HAB {
		prefix = "DEV:"
	}

	// Use a counter from the RPMB as a key diversifier to ensure we get a fresh
	// key whenever the device is wiped.
	var counter uint32
	if status.HAB {
		if err := syscall.Call("RPC.ReadIdentityCounterRPMB", nil, &counter); err != nil {
			log.Fatalf("Failed to read identity counter in RPMB, %v", err)
		}
	}

	witnessSigningKey, witnessPublicKey = deriveNoteSigner(
		fmt.Sprintf("%s%d", prefix, counter),
		status.Serial,
		func(rnd io.Reader) string {
			return fmt.Sprintf("%sArmoredWitness-%s", prefix, randomName(rnd))
		})
}

// deriveNoteSigner uses the h/w secret to derive a new note.Signer.
//
// diversifier should uniquely specify the key's intended usage, uniqueID
// should be the device's h/w unique identifier, and keyName should be a
// function which will return the name for the key - it may use the provided
// Reader as a source of entropy while generating the name if needed.
func deriveNoteSigner(diversifier string, uniqueID string, keyName func(io.Reader) string) (string, string) {
	// We'll use the provided RPC call to do the derivation in h/w, but since this is based on
	// AES it expects the diversifier to be 16 bytes long.
	// We'll hash our diversifier text and truncate to 16 bytes, and use that:
	diversifierHash := sha256.Sum256([]byte(diversifier))
	var aesKey [sha256.Size]byte
	if err := syscall.Call("RPC.DeriveKey", ([aes.BlockSize]byte)(diversifierHash[:aes.BlockSize]), &aesKey); err != nil {
		log.Fatalf("Failed to derive h/w key, %v", err)
	}

	r := hkdf.New(sha256.New, aesKey[:], []byte(uniqueID), nil)

	// And finally generate our note keypair
	sec, pub, err := note.GenerateKey(r, keyName(r))
	if err != nil {
		log.Fatalf("Failed to generate derived note key: %v", err)
	}
	return sec, pub
}

// randomName generates a random human-friendly name.
func randomName(rnd io.Reader) string {
	// Figure out our name
	nSeed := make([]byte, 8)
	if _, err := rnd.Read(nSeed); err != nil {
		log.Fatalf("Failed to read name entropy: %v", err)
	}

	ng := namegenerator.NewNameGenerator(int64(binary.LittleEndian.Uint64(nSeed)))
	return ng.Generate()
}
