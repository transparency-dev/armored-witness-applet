// Copyright (c) WithSecure Corporation
// https://foundry.withsecure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

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
