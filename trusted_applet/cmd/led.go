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
	"regexp"

	"golang.org/x/term"

	"github.com/transparency-dev/armored-witness-os/api/rpc"
	"github.com/usbarmory/GoTEE/syscall"
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
