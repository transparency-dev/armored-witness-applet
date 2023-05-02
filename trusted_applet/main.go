// Copyright (c) WithSecure Corporation
// https://foundry.withsecure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package main

import (
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/usbarmory/GoTEE/applet"
	"github.com/usbarmory/GoTEE/syscall"

	"github.com/transparency-dev/armored-witness-applet/trusted_applet/cmd"
	"github.com/transparency-dev/armored-witness-os/api"
	"github.com/transparency-dev/armored-witness-os/api/rpc"
)

var (
	Build    string
	Revision string
	Version  string

	cfg *api.Configuration
)

func init() {
	log.SetFlags(log.Ltime)
	log.SetOutput(os.Stdout)
}

func main() {
	defer applet.Exit()

	log.Printf("%s/%s (%s) • TEE user applet • %s %s",
		runtime.GOOS, runtime.GOARCH, runtime.Version(),
		Revision, Build)

	// Verify if we are allowed to run on this unit by sending version
	// information for rollback protection check.
	if err := syscall.Call("RPC.Version", Version, nil); err != nil {
		log.Fatalf("TA version check error, %v", err)
	}

	// Set default configuration, the applet is reponsible of implementing
	// its own configuration storage strategy.
	cfg = &api.Configuration{
		MAC:      MAC,
		IP:       IP,
		Netmask:  Netmask,
		Gateway:  Gateway,
		Resolver: Resolver,
	}

	// Send network configuration to Trusted OS for network initialization.
	//
	// The same call can also be used to receive configuration updates from
	// the Trusted OS, this gives the opportunity to check if the returned
	// configuration is identical to the stored one or different (meaning
	// that a configuration update has been requested through the control
	// interface).
	//
	// In this example the sent configuration is always updated with the
	// received one.
	if err := syscall.Call("RPC.Config", *cfg, cfg); err != nil {
		log.Fatalf("TA configuration error, %v", err)
	}

	log.Printf("TA Version:%s MAC:%s IP:%s GW:%s DNS:%s", Version, cfg.MAC, cfg.IP, cfg.Gateway, cfg.Resolver)

	var status api.Status

	if err := syscall.Call("RPC.Status", nil, &status); err != nil {
		log.Fatalf("TA status error, %v", err)
	}

	for _, line := range strings.Split(status.Print(), "\n") {
		log.Print(line)
	}

	ledStatus := rpc.LEDStatus{
		Name: "blue",
		On:   true,
	}

	syscall.Call("RPC.LED", ledStatus, nil)

	if err := startNetworking(); err != nil {
		log.Fatalf("TA could not initialize networking, %v", err)
	}

	listener, err := iface.ListenerTCP4(22)

	if err != nil {
		log.Fatalf("TA could not initialize SSH listener, %v", err)
	}

	go startSSHServer(listener, cfg.IP, 22, cmd.Console)

	// never returns
	eventHandler()
}
