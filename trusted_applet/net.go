// Copyright (c) WithSecure Corporation
// https://foundry.withsecure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"time"

	"gvisor.dev/gvisor/pkg/bufferv2"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/stack"

	"github.com/miekg/dns"
	"golang.org/x/term"

	"github.com/usbarmory/GoTEE/syscall"
	"github.com/usbarmory/imx-enet"

	"github.com/usbarmory/armory-witness/trusted_applet/cmd"
)

// default Trusted Applet network settings
const (
	MAC      = "1a:55:89:a2:69:41"
	IP       = "10.0.0.1"
	Netmask  = "255.255.255.0"
	Gateway  = "10.0.0.2"
	Resolver = "8.8.8.8:53"
)

// Trusted OS syscalls
const (
	RX   = 0x10000000
	TX   = 0x10000001
	FIQ  = 0x10000002
	FREQ = 0x10000003
)

var iface *enet.Interface

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

func resolve(s string) (r *dns.Msg, rtt time.Duration, err error) {
	if s[len(s)-1:] != "." {
		s += "."
	}

	msg := new(dns.Msg)
	msg.Id = dns.Id()
	msg.RecursionDesired = true

	msg.Question = make([]dns.Question, 1)
	msg.Question[0] = dns.Question{s, dns.TypeANY, dns.ClassINET}

	conn := new(dns.Conn)

	if conn.Conn, err = iface.DialTCP4(cfg.Resolver); err != nil {
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

func startNetworking() (err error) {
	if iface, err = enet.Init(nil, cfg.IP, cfg.Netmask, cfg.MAC, cfg.Gateway, 1); err != nil {
		return
	}

	iface.EnableICMP()
	iface.Link.AddNotify(&txNotification{})

	return
}
