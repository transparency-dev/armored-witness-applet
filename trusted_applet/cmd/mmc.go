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
