// Copyright (c) WithSecure Corporation
// https://foundry.withsecure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package cmd

import (
	"regexp"

	"golang.org/x/term"

	"github.com/usbarmory/GoTEE/syscall"
	"github.com/usbarmory/armory-witness/internal/rpc"
)

func init() {
	Add(Cmd{
		Name:    "led",
		Args:    2,
		Pattern: regexp.MustCompile(`^led (white|blue|yellow|green) (on|off)$`),
		Syntax:  "(white|blue|yellow|green) (on|off)",
		Help:    "LED control",
		Fn:      ledCmd,
	})
}

func ledCmd(_ *term.Terminal, arg []string) (res string, err error) {
	ledStatus := rpc.LEDStatus{
		Name: arg[0],
		On:   arg[1] == "on",
	}

	err = syscall.Call("RPC.LED", ledStatus, nil)

	return
}
