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

package cmd

import (
	"encoding/hex"
	"fmt"
	"regexp"

	"golang.org/x/term"

	"github.com/usbarmory/GoTEE/syscall"
)

const warning = `
████████████████████████████████████████████████████████████████████████████████

                                **  WARNING  **

This command activates Secure Boot, fully converting the device to exclusive
operation with executables authenticated with public keys matching the passed
SRK hash.

Secure Boot activation is an **irreversible** action that permanently fuses
values on the device. This means that you will be able to *only* execute
executable signed with matching private keys after programming is completed.

The use of this command is therefore **at your own risk**.

████████████████████████████████████████████████████████████████████████████████
`

func init() {
	Add(Cmd{
		Name:    "hab",
		Args:    1,
		Pattern: regexp.MustCompile(`^hab ([[:xdigit:]]+)$`),
		Syntax:  "<hex SRK hash>",
		Help:    "secure boot activation (*irreversible*)",
		Fn:      habCmd,
	})
}

func habCmd(term *term.Terminal, arg []string) (res string, err error) {
	fmt.Fprintf(term, "%s\n", warning)

	if !confirm(term) {
		return
	}

	srk, err := hex.DecodeString(arg[0])

	if err != nil {
		return
	}

	return "", syscall.Call("RPC.HAB", srk, nil)
}
