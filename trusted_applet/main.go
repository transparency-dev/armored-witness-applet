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

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/transparency-dev/armored-witness-applet/trusted_applet/cmd"
	"github.com/transparency-dev/armored-witness-os/api"
	"github.com/transparency-dev/armored-witness-os/api/rpc"
	"github.com/usbarmory/GoTEE/applet"
	"github.com/usbarmory/GoTEE/syscall"
	"google.golang.org/protobuf/proto"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
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
		DHCP:     DHCP,
		IP:       IP,
		Netmask:  Netmask,
		Gateway:  Gateway,
		Resolver: DefaultResolver,
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
	var cfgResp []byte
	if err := syscall.Call("RPC.Config", cfg.Bytes(), &cfgResp); err != nil {
		log.Printf("TA configuration error, %v", err)
	}

	if cfgResp != nil {
		if err := proto.Unmarshal(cfgResp, cfg); err != nil {
			log.Fatalf("TA configuration invalid: %v", err)
		}
	}
	var status api.Status

	if err := syscall.Call("RPC.Status", nil, &status); err != nil {
		log.Fatalf("TA status error, %v", err)
	}

	for _, line := range strings.Split(status.Print(), "\n") {
		log.Print(line)
	}

	syscall.Call("RPC.LED", rpc.LEDStatus{Name: "blue", On: true}, nil)
	defer syscall.Call("RPC.LED", rpc.LEDStatus{Name: "blue", On: false}, nil)

	if err := startNetworking(); err != nil {
		log.Fatalf("TA could not initialize networking, %v", err)
	}

	// Register and run our RPC handler so we can receive ethernet frames.
	go eventHandler()

	ctx := context.Background()
	// Wait for a DHCP address to be assigned if that's what we're configured to do
	if cfg.DHCP {
		runDHCP(ctx, nicID, runWithNetworking)
	} else {
		if err := runWithNetworking(ctx); err != nil && err != context.Canceled {
			log.Printf("runWithNetworking: %v", err)
		}
	}
}

// runWithNetworking should only be called when we have an IP network configured.
// ctx should become Done if the network becomes unconfigured for any reason (e.g.
// DHCP lease expires).
//
// Everything which relies on IP networking being present should be started in
// here, and should gracefully stop when the passed-in context is Done.
func runWithNetworking(ctx context.Context) error {
	addr, tcpErr := iface.Stack.GetMainNICAddress(nicID, ipv4.ProtocolNumber)
	if tcpErr != nil {
		return fmt.Errorf("runWithNetworking has no network configured: %v", tcpErr)
	}
	log.Printf("TA Version:%s MAC:%s IP:%s GW:%s DNS:%s", Version, iface.NIC.MAC.String(), addr, iface.Stack.GetRouteTable(), resolver)

	listener, err := iface.ListenerTCP4(22)
	if err != nil {
		return fmt.Errorf("TA could not initialize SSH listener, %v", err)
	}
	defer func() {
		log.Println("Closing ssh port")
		if err := listener.Close(); err != nil {
			log.Printf("Error closing ssh port: %v", err)
		}
	}()

	go startSSHServer(ctx, listener, addr.Address.String(), 22, cmd.Console)

	<-ctx.Done()
	return ctx.Err()
}
