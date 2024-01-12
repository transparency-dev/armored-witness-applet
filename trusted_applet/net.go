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
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"

	"github.com/beevik/ntp"
	"github.com/transparency-dev/armored-witness-applet/third_party/dhcp"
	"github.com/transparency-dev/armored-witness-os/api"
	"golang.org/x/term"

	"github.com/usbarmory/GoTEE/applet"
	"github.com/usbarmory/GoTEE/syscall"
	enet "github.com/usbarmory/imx-enet"

	"github.com/transparency-dev/armored-witness-applet/trusted_applet/cmd"
)

// default Trusted Applet network settings
const (
	DHCP            = true
	IP              = "10.0.0.1"
	Netmask         = "255.255.255.0"
	Gateway         = "10.0.0.2"
	DefaultResolver = "8.8.8.8:53"
	DefaultNTP      = "time.google.com"

	nicID = tcpip.NICID(1)

	// Timeout for any http requests.
	httpTimeout = 30 * time.Second
)

// Trusted OS syscalls
const (
	RX   = 0x10000000
	TX   = 0x10000001
	FIQ  = 0x10000002
	FREQ = 0x10000003
)

var (
	iface *enet.Interface
)

func init() {
	cmd.Add(cmd.Cmd{
		Name:    "dns",
		Args:    1,
		Pattern: regexp.MustCompile(`^dns (.*)`),
		Syntax:  "<fqdn>",
		Help:    "resolve domain (requires routing)",
		Fn:      dnsCmd,
	})

	net.DefaultNS = []string{DefaultResolver}
}

// runDHCP starts the dhcp client.
//
// When an IP is successfully leased and configured on the interface, f is called with a context
// which will become Done when the leased address expires. Callers can use this as a mechanism to
// ensure that networking clients/services are only run while a leased IP is held.
//
// This function blocks until the passed-in ctx is Done.
func runDHCP(ctx context.Context, nicID tcpip.NICID, f func(context.Context) error) {
	// This context tracks the lifetime of the IP lease we get (if any) from the DHCP server.
	// We'll only know what that lease is once we acquire the new IP, which happens inside
	// the aquired func below.
	var (
		childCtx    context.Context
		cancelChild context.CancelFunc
	)
	// fDone is used to ensure that we wait for the passed-in func f to complete before
	// make changes to the network stack or attempt to rerun f when we've acquired a new lease.
	fDone := make(chan bool, 1)
	defer close(fDone)

	// acquired handles our dhcp.Client events - acquiring, releasing, renewing DHCP leases.
	acquired := func(oldAddr, newAddr tcpip.AddressWithPrefix, cfg dhcp.Config) {
		log.Printf("DHCPC: lease update - old: %v, new: %v", oldAddr.String(), newAddr.String())
		// Handled renewals first, old and new addresses will be equivalent in this case.
		// We may still have to reconfigure the networking stack, even though our assigned IP
		// isn't changing, the DHCP server could have changed routing or DNS info.
		if oldAddr.Address == newAddr.Address && oldAddr.PrefixLen == newAddr.PrefixLen {
			log.Printf("DHCPC: existing lease on %v renewed", newAddr.String())
			// reconfigure network stuff in-case DNS or gateway routes have changed.
			configureNetFromDHCP(newAddr, cfg)
			// f should already be running, no need to interfere with it.
			return
		}

		// If oldAddr is specified, then our lease on that address has experied - remove it
		// from our stack.
		if !oldAddr.Address.Unspecified() {
			// Since we're changing our primary IP address we must tell f to exit,
			// and wait for it to do so
			cancelChild()
			log.Print("Waiting for child to complete...")
			<-fDone

			log.Printf("DHCPC: Releasing %v", oldAddr.String())
			if err := iface.Stack.RemoveAddress(nicID, oldAddr.Address); err != nil {
				log.Printf("Failed to remove expired address from stack: %v", err)
			}
		}

		// If newAddr is specified, then we've been granted a lease on a new IP address, so
		// we'll configure our stack to use it, along with whatever routes and DNS info
		// we've been sent.
		if !newAddr.Address.Unspecified() {
			log.Printf("DHCPC: Acquired %v", newAddr.String())

			newProtoAddr := tcpip.ProtocolAddress{
				Protocol:          ipv4.ProtocolNumber,
				AddressWithPrefix: newAddr,
			}
			if err := iface.Stack.AddProtocolAddress(nicID, newProtoAddr, stack.AddressProperties{PEB: stack.FirstPrimaryEndpoint}); err != nil {
				log.Printf("Failed to add newly acquired address to stack: %v", err)
			} else {
				configureNetFromDHCP(newAddr, cfg)

				// Set up a context we'll use to control f's execution lifetime.
				// This will get canceled above if/when our IP lease expires.
				childCtx, cancelChild = context.WithCancel(ctx)

				// And execute f in its own goroutine so we don't block the dhcp.Client.
				go func(childCtx context.Context) {
					// Signal when we exit:
					defer func() { fDone <- true }()

					log.Println("DHCP: running f")
					if err := f(childCtx); err != nil {
						log.Printf("runDHCP f: %v", err)
					}
					log.Println("DHCP: f exited")
				}(childCtx)
			}
		} else {
			log.Printf("DHCPC: no address acquired")
		}
	}

	// Start the DHCP client.
	c := dhcp.NewClient(iface.Stack, nicID, iface.Link.LinkAddress(), 30*time.Second, time.Second, time.Second, acquired)
	log.Println("Starting DHCPClient...")
	c.Run(ctx)
}

// configureNetFromDHCP sets up network related configuration, e.g. DNS servers,
// gateway routes, etc. based on config received from the DHCP server.
// Note that this function does not update the network stack's assigned IP address.
func configureNetFromDHCP(newAddr tcpip.AddressWithPrefix, cfg dhcp.Config) {
	if len(cfg.DNS) > 0 {
		resolvers := []string{}
		for _, r := range cfg.DNS {
			resolver := fmt.Sprintf("%s:53", r.String())
			resolvers = append(resolvers, resolver)
		}
		log.Printf("DHCPC: Using DNS server(s) %v", resolvers)
		net.DefaultNS = resolvers
	}
	// Set up routing for new address
	// Start with the implicit route to local segment
	table := []tcpip.Route{
		{Destination: newAddr.Subnet(), NIC: nicID},
	}
	// add any additional routes from the DHCP server
	if len(cfg.Router) > 0 {
		for _, gw := range cfg.Router {
			table = append(table, tcpip.Route{Destination: header.IPv4EmptySubnet, Gateway: gw, NIC: nicID})
			log.Printf("DHCPC: Using Gateway %v", gw)
		}
	}
	iface.Stack.SetRouteTable(table)
}

// runNTP starts periodically attempting to sync the system time with NTP.
// Returns a channel which become closed once we have obtained an initial time.
func runNTP(ctx context.Context) chan bool {
	if cfg.NTPServer == "" {
		log.Println("NTP disabled.")
		return nil
	}

	r := make(chan bool)

	go func(ctx context.Context) {
		// i specifies the interval between checking in with the NTP server.
		// Initially we'll check in more frequently until we have set a time.
		i := time.Second * 10
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(i):
			}

			ip, err := net.DefaultResolver.LookupIP(ctx, "ip4", cfg.NTPServer)
			if err != nil {
				log.Printf("Failed to resolve NTP server %q: %v", DefaultNTP, err)
				continue
			}
			ntpR, err := ntp.QueryWithOptions(
				ip[0].String(),
				ntp.QueryOptions{},
			)
			if err != nil {
				log.Printf("Failed to get NTP time: %v", err)
				continue
			}
			if err := ntpR.Validate(); err != nil {
				log.Printf("got invalid time from NTP server: %v", err)
				continue
			}
			applet.ARM.SetTimer(ntpR.Time.UnixNano())

			// We've got some sort of sensible time set now, so check in with NTP
			// much less frequently.
			i = time.Hour
			if r != nil {
				// Signal that we've got an initial time.
				close(r)
				r = nil
			}
		}
	}(ctx)

	return r
}

func dnsCmd(_ *term.Terminal, arg []string) (res string, err error) {
	if iface == nil {
		return "", errors.New("network is unavailable")
	}

	r, err := net.LookupHost(arg[0])

	if err != nil {
		return fmt.Sprintf("query error: %v", err), nil
	}
	if len(r) == 0 {
		return "", errors.New("no results returned")
	}

	return strings.Join(r, ", "), nil
}

func rxFromEth(buf []byte) int {
	n := syscall.Read(RX, buf, uint(len(buf)))

	if n == 0 || n > int(enet.MTU) {
		return 0
	}

	return n
}

func rx(buf []byte) {
	if len(buf) < 14 {
		return
	}

	hdr := buf[0:14]
	proto := tcpip.NetworkProtocolNumber(binary.BigEndian.Uint16(buf[12:14]))
	payload := buf[14:]

	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		ReserveHeaderBytes: len(hdr),
		Payload:            buffer.MakeWithData(payload),
	})

	copy(pkt.LinkHeader().Push(len(hdr)), hdr)

	iface.Link.InjectInbound(proto, pkt)
}

func tx() (buf []byte) {
	var pkt stack.PacketBufferPtr

	if pkt = iface.NIC.Link.Read(); pkt.IsNil() {
		return
	}

	proto := make([]byte, 2)
	binary.BigEndian.PutUint16(proto, uint16(pkt.NetworkProtocolNumber))

	// Ethernet frame header
	buf = append(buf, pkt.EgressRoute.RemoteLinkAddress...)
	buf = append(buf, iface.NIC.MAC...)
	buf = append(buf, proto...)

	for _, v := range pkt.AsSlices() {
		buf = append(buf, v...)
	}

	return
}

type txNotification struct{}

func (n *txNotification) WriteNotify() {
	buf := tx()
	syscall.Write(TX, buf, uint(len(buf)))
}

// mac creates a stable "local administered" MAC address for the network based on the
// provided unit serial number.
func mac(serial string) string {
	m := sha256.Sum256([]byte(fmt.Sprintf("MAC:%s", serial)))
	// The first byte of the MAC address has a couple of flags which must be set correctly:
	// - Unicast(0)/multicast(1) in the least significant bit of the byte.
	//   This must be set to unicast.
	// - Universally unique(0)/Local administered(1) in the second least significant bit.
	//   Since we're not using an organisationally unique prefix triplet, this must be set to
	//   Locally administered
	m[0] &= 0xfe
	m[0] |= 0x02
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", m[0], m[1], m[2], m[3], m[4], m[5])
}

func startNetworking() (err error) {
	// Set the default resolver from the config, if we're using DHCP this may be updated.
	net.DefaultNS = []string{cfg.Resolver}

	var status api.Status
	if err := syscall.Call("RPC.Status", nil, &status); err != nil {
		return fmt.Errorf("failed to fetch Status: %v", err)
	}

	if iface, err = enet.Init(nil, cfg.IP, cfg.Netmask, mac(status.Serial), cfg.Gateway, int(nicID)); err != nil {
		return
	}

	iface.EnableICMP()
	iface.Link.AddNotify(&txNotification{})

	// hook interface into Go runtime
	net.SocketFunc = iface.Socket

	http.DefaultClient.Timeout = httpTimeout

	return
}
