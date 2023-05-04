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
	"bytes"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"golang.org/x/term"

	"github.com/usbarmory/GoTEE/applet"
	"github.com/usbarmory/GoTEE/syscall"

	"github.com/transparency-dev/armored-witness-os/api"
)

func init() {
	Add(Cmd{
		Name: "help",
		Help: "this help",
		Fn:   helpCmd,
	})

	Add(Cmd{
		Name:    "exit, quit",
		Args:    1,
		Pattern: regexp.MustCompile(`^(exit|quit)$`),
		Help:    "close session",
		Fn:      exitCmd,
	})

	Add(Cmd{
		Name: "stack",
		Help: "stack trace of current goroutine",
		Fn:   stackCmd,
	})

	Add(Cmd{
		Name: "stackall",
		Help: "stack trace of all goroutines",
		Fn:   stackallCmd,
	})

	Add(Cmd{
		Name:    "date",
		Args:    1,
		Pattern: regexp.MustCompile(`^date(.*)`),
		Syntax:  "(time in RFC339 format)?",
		Help:    "show/change runtime date and time",
		Fn:      dateCmd,
	})

	Add(Cmd{
		Name: "status",
		Help: "status information",
		Fn:   statusCmd,
	})

	Add(Cmd{
		Name: "reboot",
		Help: "reset device",
		Fn:   rebootCmd,
	})
}

func helpCmd(term *term.Terminal, _ []string) (string, error) {
	return Help(term), nil
}

func exitCmd(_ *term.Terminal, _ []string) (string, error) {
	return "logout", io.EOF
}

func stackCmd(_ *term.Terminal, _ []string) (string, error) {
	return string(debug.Stack()), nil
}

func stackallCmd(_ *term.Terminal, _ []string) (string, error) {
	buf := new(bytes.Buffer)
	pprof.Lookup("goroutine").WriteTo(buf, 1)

	return buf.String(), nil
}

func dateCmd(_ *term.Terminal, arg []string) (res string, err error) {
	if len(arg[0]) > 1 {
		t, err := time.Parse(time.RFC3339, arg[0][1:])

		if err != nil {
			return "", err
		}

		applet.ARM.SetTimer(t.UnixNano())
	}

	return fmt.Sprintf("%s", time.Now().Format(time.RFC3339)), nil
}

func statusCmd(_ *term.Terminal, _ []string) (info string, err error) {
	var res bytes.Buffer
	var tee api.Status

	res.WriteString("------------------------------------------------------- Trusted Applet ----\n")
	res.WriteString(fmt.Sprintf("Runtime ......: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH))

	if err = syscall.Call("RPC.Status", nil, &tee); err != nil {
		return
	}

	res.WriteString(tee.Print())

	return res.String(), nil
}

func rebootCmd(_ *term.Terminal, _ []string) (_ string, _ error) {
	return "", syscall.Call("RPC.Reboot", nil, nil)
}
