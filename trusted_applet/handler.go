// Copyright (c) WithSecure Corporation
// https://foundry.withsecure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package main

import (
	"log"
	"math"
	"runtime"
	"time"

	"github.com/usbarmory/GoTEE/syscall"
	"github.com/usbarmory/imx-enet"

	"github.com/usbarmory/armory-witness/internal/rpc"
)

func eventHandler() {
	var handler rpc.Handler

	handler.G, handler.P = runtime.GetG()

	if err := syscall.Call("RPC.Register", handler, nil); err != nil {
		log.Fatalf("TA event handler registration error, %v", err)
	}

	n := 0
	out := make([]byte, enet.MTU)

	for {
		// To avoid losing interrupts, re-enabling must happen only
		// after we are sleeping.
		go syscall.Write(FIQ, nil, 0)

		// sleep indefinitely until woken up by runtime.WakeG
		time.Sleep(math.MaxInt64)

		// check for Ethernet RX event
		for n = rxFromEth(out); n > 0; n = rxFromEth(out) {
			rx(out[0:n])
		}
	}
}
