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
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/usbarmory/GoTEE/applet"
	"github.com/usbarmory/GoTEE/syscall"
	"github.com/usbarmory/tamago/soc/nxp/usdhc"
	"google.golang.org/protobuf/proto"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"

	"github.com/transparency-dev/armored-witness-applet/trusted_applet/cmd"
	"github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/storage"
	"github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/storage/slots"
	"github.com/transparency-dev/armored-witness-os/api"
	"github.com/transparency-dev/armored-witness-os/api/rpc"

	"github.com/golang/glog"
	"github.com/transparency-dev/witness/omniwitness"
	"golang.org/x/mod/sumdb/note"
)

const (
	// Generated from https://go.dev/play/p/uWUKLNK6h9v
	// TODO(mhutchinson): these need to be read from file instead of constants
	publicKey  = "TrustMe+68958214+AQ4Ys/PsXqfhPkNK7Y7RyYUMOJvfl65PzJOEiq9VFPjF"
	signingKey = "PRIVATE+KEY+TrustMe+68958214+AZKby3TDZizdARF975ZyLJwGbHTivd+EqbfYTN5qr2cI"

	// slotsPartitionOffsetBytes defines where our witness data storage partition starts.
	// Changing this location is overwhelmingly likely to result in data loss.
	slotsPartitionOffsetBytes = 1 << 30
	// slotsPartitionLengthBytes specifies the size of the slots partition.
	// Increasing this value is relatively safe, if you're sure there is no data
	// stored in blocks which follow the current partition.
	//
	// We're starting with enough space for 1024 slots of 1MB each.
	slotsPartitionLengthBytes = 1024 * slotSizeBytes

	// slotSizeBytes is the size of each individual slot in the partition.
	slotSizeBytes = 1 << 20
)

var (
	Build    string
	Revision string
	Version  string

	cfg *api.Configuration

	persistence *storage.SlotPersistence
)

func init() {
	log.SetFlags(log.Ltime)
	log.SetOutput(os.Stdout)
}

func main() {
	flag.Set("vmodule", "journal=1,slots=1,storage=1")
	flag.Set("v", "1")
	flag.Set("logtostderr", "true")
	flag.Parse()

	ctx := context.Background()
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
		DHCP:      DHCP,
		IP:        IP,
		Netmask:   Netmask,
		Gateway:   Gateway,
		Resolver:  DefaultResolver,
		NTPServer: DefaultNTP,
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

	if len(cfgResp) > 0 {
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

	go func() {
		l := true
		for {
			syscall.Call("RPC.LED", rpc.LEDStatus{Name: "blue", On: l}, nil)
			l = !l
			time.Sleep(500 * time.Millisecond)
		}
	}()

	if err := startNetworking(); err != nil {
		log.Fatalf("TA could not initialize networking, %v", err)
	}

	syscall.Call("RPC.Address", iface.NIC.MAC, nil)

	// Register and run our RPC handler so we can receive ethernet frames.
	go eventHandler()

	glog.Infof("Opening storage...")
	part := openStorage()
	glog.Infof("Storage opened.")

	// Set this to true to "wipe" the storage.
	// Currently this simply force-writes an entry with zero bytes to
	// each known slot.
	// If the journal(s) become corrupt a larger hammer will be required.
	reinit := false
	if reinit {
		for i := 10; i > 0; i-- {
			log.Printf("Erasing in %ds", i)
			time.Sleep(time.Second)
		}
		if err := part.Erase(); err != nil {
			glog.Exitf("Failed to erase partition: %v", err)
		}
		glog.Exit("Erase completed")
	}

	persistence = storage.NewSlotPersistence(part)
	if err := persistence.Init(); err != nil {
		glog.Exitf("Failed to create persistence layer: %v", err)
	}
	persistence.Init()

	// Wait for a DHCP address to be assigned if that's what we're configured to do
	if cfg.DHCP {
		runDHCP(ctx, nicID, runWithNetworking)
	} else {
		if err := runWithNetworking(ctx); err != nil && err != context.Canceled {
			glog.Exitf("runWithNetworking: %v", err)
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

	<-runNTP(ctx)

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

	// Set up and start omniwitness
	httpClient := getHttpClient()

	signer, err := note.NewSigner(signingKey)
	if err != nil {
		return fmt.Errorf("failed to init signer: %v", err)
	}
	verifier, err := note.NewVerifier(publicKey)
	if err != nil {
		return fmt.Errorf("failed to init verifier: %v", err)
	}
	opConfig := omniwitness.OperatorConfig{
		WitnessSigner:   signer,
		WitnessVerifier: verifier,
	}
	// TODO(mhutchinson): add a second listener for an admin API.
	mainListener, err := iface.ListenerTCP4(80)
	if err != nil {
		return fmt.Errorf("could not initialize HTTP listener: %v", err)
	}

	log.Println("Starting witness...")
	if err := omniwitness.Main(ctx, opConfig, persistence, mainListener, httpClient); err != nil {
		return fmt.Errorf("omniwitness.Main failed: %v", err)
	}

	return ctx.Err()
}

func openStorage() *slots.Partition {
	var info usdhc.CardInfo
	if err := syscall.Call("RPC.CardInfo", nil, &info); err != nil {
		glog.Exitf("Failed to get cardinfo: %v", err)
	}
	log.Printf("CardInfo: %+v", info)
	// dev is our access to the MMC storage.
	dev := &storage.Device{CardInfo: &info}
	bs := dev.BlockSize()
	geo := slots.Geometry{
		Start:  slotsPartitionOffsetBytes / bs,
		Length: slotsPartitionLengthBytes / bs,
	}
	sl := slotSizeBytes / bs
	for i := uint(0); i < geo.Length; i += sl {
		geo.SlotLengths = append(geo.SlotLengths, sl)
	}

	p, err := slots.OpenPartition(dev, geo)
	if err != nil {
		glog.Exitf("Failed to open partition: %v", err)
	}
	return p
}
