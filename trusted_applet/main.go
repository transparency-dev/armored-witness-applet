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
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

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
	"github.com/transparency-dev/witness/monitoring"
	"github.com/transparency-dev/witness/monitoring/prometheus"
	"github.com/transparency-dev/witness/omniwitness"
	"golang.org/x/mod/sumdb/note"

	_ "golang.org/x/crypto/x509roots/fallback"
)

const (
	// slotsPartitionStartBlock defines where our witness data storage partition starts.
	// Changing this location is overwhelmingly likely to result in data loss.
	slotsPartitionStartBlock = 0x400000
	// slotsPartitionLengthBlocks specifies the size of the slots partition.
	// Increasing this value is relatively safe, if you're sure there is no data
	// stored in blocks which follow the current partition.
	//
	// We're starting with enough space for 4096 slots of 512KB each, which should be plenty.
	slotsPartitionLengthBlocks = 0x400000

	// slotSizeBytes is the size of each individual slot in the partition.
	// Changing this is overwhelmingly likely to result in data loss.
	slotSizeBytes = 512 << 10
)

var (
	Build    string
	Revision string
	Version  string

	GitHubUser, GitHubEmail, GitHubToken string
	RestDistributorBaseURL               string

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

	mf := prometheus.MetricFactory{
		Prefix: "omniwitness_",
	}
	monitoring.SetMetricFactory(mf)

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

	// (Re-)create our witness identity based on the device's internal secret key.
	deriveWitnessKey()
	// Update our status in OS so custodian can inspect our signing identity even if there's no network.
	syscall.Call("RPC.SetWitnessStatus", rpc.WitnessStatus{Identity: witnessPublicKey}, nil)

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
	// Update status with latest IP address too.
	syscall.Call("RPC.SetWitnessStatus", rpc.WitnessStatus{Identity: witnessPublicKey, IP: addr.Address.String()}, nil)

	select {
	case <-runNTP(ctx):
	case <-ctx.Done():
		return ctx.Err()
	}

	time.Sleep(5 * time.Second)

	listenCfg := &net.ListenConfig{}

	sshListener, err := listenCfg.Listen(ctx, "tcp", ":22")
	if err != nil {
		return fmt.Errorf("TA could not initialize SSH listener, %v", err)
	}
	defer func() {
		log.Println("Closing ssh port")
		if err := sshListener.Close(); err != nil {
			log.Printf("Error closing ssh port: %v", err)
		}
	}()
	go startSSHServer(ctx, sshListener, addr.Address.String(), 22, cmd.Console)

	metricsListener, err := listenCfg.Listen(ctx, "tcp", ":8081")
	if err != nil {
		return fmt.Errorf("TA could not initialize metrics listener, %v", err)
	}
	defer func() {
		log.Println("Closing metrics port (8081)")
		if err := metricsListener.Close(); err != nil {
			log.Printf("Error closing ssh port: %v", err)
		}
	}()
	go func() {
		srvMux := http.NewServeMux()
		srvMux.Handle("/metrics", promhttp.Handler())
		srv := &http.Server{
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			Handler:      srvMux,
		}
		if err := srv.Serve(metricsListener); err != http.ErrServerClosed {
			glog.Errorf("Error serving metrics: %v", err)
		}
	}()

	// Set up and start omniwitness
	signer, err := note.NewSigner(witnessSigningKey)
	if err != nil {
		return fmt.Errorf("failed to init signer: %v", err)
	}
	verifier, err := note.NewVerifier(witnessPublicKey)
	if err != nil {
		return fmt.Errorf("failed to init verifier: %v", err)
	}
	opConfig := omniwitness.OperatorConfig{
		WitnessSigner:          signer,
		WitnessVerifier:        verifier,
		GithubUser:             GitHubUser,
		GithubEmail:            GitHubEmail,
		GithubToken:            GitHubToken,
		RestDistributorBaseURL: RestDistributorBaseURL,
	}
	// TODO(mhutchinson): add a second listener for an admin API.
	mainListener, err := listenCfg.Listen(ctx, "tcp", ":80")
	if err != nil {
		return fmt.Errorf("could not initialize HTTP listener: %v", err)
	}
	defer func() {
		if err := mainListener.Close(); err != nil {
			log.Printf("mainListener: %v", err)
		}
	}()

	log.Println("Starting witness...")
	log.Printf("I am %q", witnessPublicKey)
	if err := omniwitness.Main(ctx, opConfig, persistence, mainListener, http.DefaultClient); err != nil {
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
		Start:  slotsPartitionStartBlock,
		Length: slotsPartitionLengthBlocks,
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
