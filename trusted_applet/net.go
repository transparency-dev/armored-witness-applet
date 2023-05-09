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
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"regexp"
	"time"

	"gvisor.dev/gvisor/pkg/bufferv2"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"

	"github.com/miekg/dns"
	"github.com/transparency-dev/armored-witness-applet/third_party/dhcp"
	"golang.org/x/term"

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

	nicID = tcpip.NICID(1)
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

	// resolver is the DNS server address:port to use to resolve names
	resolver string
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
}

// runDHCP starts the dhcp client.
//
// When an IP is successfully leased and configured on the interface, f is called with a context
// which will become Done when the leased address expires. Callers can use this as a mechanism to
// ensure that networking clients/services are only run while a leased IP is held.
//
// This function blocks until the passed-in ctx is Done.
func runDHCP(ctx context.Context, nicID tcpip.NICID, f func(context.Context) error) {
	childCtx, cancelChild := context.WithCancel(ctx)

	acquired := func(oldAddr, newAddr tcpip.AddressWithPrefix, cfg dhcp.Config) {
		log.Printf("DHCPC: lease update - old: %v, new: %v", oldAddr.String(), newAddr.String())
		if oldAddr.Address == newAddr.Address && oldAddr.PrefixLen == newAddr.PrefixLen {
			log.Printf("DHCPC: existing lease on %v renewed", newAddr.String())
			return
		}
		newProtoAddr := tcpip.ProtocolAddress{
			Protocol:          ipv4.ProtocolNumber,
			AddressWithPrefix: newAddr,
		}
		if !oldAddr.Address.Unspecified() {
			log.Printf("DHCPC: Releasing %v", oldAddr.String())
			if err := iface.Stack.RemoveAddress(nicID, oldAddr.Address); err != nil {
				log.Printf("Failed to remove expired address from stack: %v", err)
			}
			cancelChild()
		}

		if !newAddr.Address.Unspecified() {
			log.Printf("DHCPC: Acquired %v", newAddr.String())
			if err := iface.Stack.AddProtocolAddress(nicID, newProtoAddr, stack.AddressProperties{PEB: stack.FirstPrimaryEndpoint}); err != nil {
				log.Printf("Failed to add newly acquired address to stack: %v", err)
			} else {
				cancelChild()
				childCtx, cancelChild = context.WithCancel(ctx)
				if len(cfg.DNS) > 0 {
					resolver = fmt.Sprintf("%s:53", cfg.DNS[0].String())
					log.Printf("DHCPC: Using DNS server %v", resolver)
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

				go func(childCtx context.Context) {
					if err := f(childCtx); err != nil {
						log.Printf("runDHCP f: %v", err)
					}
				}(childCtx)
			}
		} else {
			log.Printf("DHCPC: no address acquired")
		}
	}

	c := dhcp.NewClient(iface.Stack, nicID, iface.Link.LinkAddress(), 30*time.Second, time.Second, time.Second, acquired)
	log.Println("Starting DHCPClient...")
	c.Run(ctx)
}

func resolve(s string) (r *dns.Msg, rtt time.Duration, err error) {
	if s[len(s)-1:] != "." {
		s += "."
	}

	msg := new(dns.Msg)
	msg.Id = dns.Id()
	msg.RecursionDesired = true

	msg.Question = make([]dns.Question, 1)
	msg.Question[0] = dns.Question{s, dns.TypeA, dns.ClassINET}

	conn := new(dns.Conn)

	if conn.Conn, err = iface.DialTCP4(resolver); err != nil {
		return
	}

	c := new(dns.Client)

	return c.ExchangeWithConn(msg, conn)
}

func dnsCmd(_ *term.Terminal, arg []string) (res string, err error) {
	if iface == nil {
		return "", errors.New("network is unavailable")
	}

	r, _, err := resolve(arg[0])

	if err != nil {
		return fmt.Sprintf("query error: %v", err), nil
	}

	return fmt.Sprintf("%+v", r), nil
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
		Payload:            bufferv2.MakeWithData(payload),
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

func mac() string {
	m := make([]uint8, 6)
	if _, err := rand.Read(m); err != nil {
		panic(fmt.Sprintf("failed to read %d bytes for randomised MAC address: %v", len(m), err))
	}
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
	resolver = cfg.Resolver

	if iface, err = enet.Init(nil, cfg.IP, cfg.Netmask, mac(), cfg.Gateway, int(nicID)); err != nil {
		return
	}

	iface.EnableICMP()
	iface.Link.AddNotify(&txNotification{})

	return
}
