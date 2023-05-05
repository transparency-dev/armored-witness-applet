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
	"sync"
	"time"

	"github.com/usbarmory/GoTEE/applet"
	"github.com/usbarmory/GoTEE/syscall"
	"google.golang.org/protobuf/proto"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"

	"github.com/transparency-dev/armored-witness-applet/trusted_applet/cmd"
	"github.com/transparency-dev/armored-witness-os/api"
	"github.com/transparency-dev/armored-witness-os/api/rpc"

	"github.com/golang/glog"
	"github.com/transparency-dev/witness/omniwitness"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Timeout for any http requests.
	httpTimeout = 10 * time.Second

	// Generated from https://go.dev/play/p/uWUKLNK6h9v
	// TODO(mhutchinson): these need to be read from file instead of constants
	publicKey  = "TrustMe+68958214+AQ4Ys/PsXqfhPkNK7Y7RyYUMOJvfl65PzJOEiq9VFPjF"
	signingKey = "PRIVATE+KEY+TrustMe+68958214+AZKby3TDZizdARF975ZyLJwGbHTivd+EqbfYTN5qr2cI"
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

	if err := startNetworking(); err != nil {
		log.Fatalf("TA could not initialize networking, %v", err)
	}

	syscall.Call("RPC.Address", iface.NIC.MAC, nil)

	// Register and run our RPC handler so we can receive ethernet frames.
	go eventHandler()

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

	// Set up and start omniwitness
	httpClient := getHttpClient()

	signer, err := note.NewSigner(signingKey)
	if err != nil {
		glog.Exitf("Failed to init signer: %v", err)
	}
	verifier, err := note.NewVerifier(publicKey)
	if err != nil {
		glog.Exitf("Failed to init verifier: %v", err)
	}
	opConfig := omniwitness.OperatorConfig{
		WitnessSigner:   signer,
		WitnessVerifier: verifier,
	}

	// TODO(mhutchinson): add a second listener for an admin API.
	mainListener, err := iface.ListenerTCP4(80)
	if err != nil {
		glog.Exitf("could not initialize HTTP listener: %v", err)
	}
	p := memPersistance{}
	p.Init()

	go func() {
		if err := omniwitness.Main(ctx, opConfig, &p, mainListener, httpClient); err != nil {
			glog.Exitf("Main failed: %v", err)
		}
	}()

	<-ctx.Done()
	return ctx.Err()
}

// Temporary in-memory witness storage implementation below.
// This will be replaced shortly.

type checkpointAndProof struct {
	checkpoint []byte
	proof      []byte
}

type memPersistance struct {
	mu sync.Mutex
	fs map[string]checkpointAndProof
}

func (m *memPersistance) Init() error {
	m.fs = make(map[string]checkpointAndProof)
	return nil
}

func (m *memPersistance) ReadOps(id string) (omniwitness.LogStateReadOps, error) {
	return ops{
		id: id,
		p:  m,
	}, nil
}

func (m *memPersistance) Logs() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	logs := make(map[string]bool)
	for k := range m.fs {
		if strings.HasPrefix(k, "/logs/") {
			bits := strings.Split(k, "/")
			logs[bits[1]] = true
		}
	}
	ret := []string{}
	for k := range logs {
		ret = append(ret, k)
	}
	return ret, nil
}

type ops struct {
	id string
	p  *memPersistance
}

func (o ops) GetLatest() ([]byte, []byte, error) {
	o.p.mu.Lock()
	defer o.p.mu.Unlock()
	l, ok := o.p.fs[fmt.Sprintf("/logs/%s/latest", o.id)]
	if !ok {
		return nil, nil, status.Error(codes.NotFound, "no checkpoint for log")
	}
	return l.checkpoint, l.proof, nil
}

func (o *memPersistance) WriteOps(id string) (omniwitness.LogStateWriteOps, error) {
	return ops{
		id: id,
		p:  o,
	}, nil
}

func (o ops) Set(checkpointRaw []byte, compactRange []byte) error {
	o.p.mu.Lock()
	o.p.fs[fmt.Sprintf("/logs/%s/latest", o.id)] = checkpointAndProof{
		checkpoint: checkpointRaw,
		proof:      compactRange,
	}
	return nil
}

func (o ops) Close() error {
	o.p.mu.Unlock()
	return nil
}
