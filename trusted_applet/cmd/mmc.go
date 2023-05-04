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
	"strconv"

	"golang.org/x/term"

	"github.com/usbarmory/GoTEE/syscall"

	"github.com/transparency-dev/armored-witness-os/api/rpc"
)

const (
	// We could use the entire iRAM before USB activation,
	// accounting for required dTD alignment which takes
	// additional space (readSize = 0x20000 - 4096).
	readSize      = 0x7fff
	totalReadSize = 10 * 1024 * 1024
)

func init() {
	Add(Cmd{
		Name:    "mmc",
		Args:    2,
		Pattern: regexp.MustCompile(`^mmc ([[:xdigit:]]+) (\d+)$`),
		Syntax:  "<hex offset> <size>",
		Help:    "MMC card read",
		Fn:      mmcCmd,
	})
}

func mmcCmd(_ *term.Terminal, arg []string) (res string, err error) {
	addr, err := strconv.ParseUint(arg[0], 16, 32)

	if err != nil {
		return "", fmt.Errorf("invalid address: %v", err)
	}

	size, err := strconv.ParseUint(arg[1], 10, 32)

	if err != nil {
		return "", fmt.Errorf("invalid size: %v", err)
	}

	if size > maxBufferSize {
		return "", fmt.Errorf("size argument must be <= %d", maxBufferSize)
	}

	buf := make([]byte, size)

	xfer := &rpc.Read{
		Offset: int64(addr),
		Size:   int64(size),
	}

	if err = syscall.Call("RPC.Read", xfer, &buf); err != nil {
		return
	}

	return hex.Dump(buf), nil
}
