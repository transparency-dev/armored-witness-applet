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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"k8s.io/klog/v2"
)

type consoleHandler func(term *term.Terminal)

func handleChannel(newChannel ssh.NewChannel, handler consoleHandler) {
	if t := newChannel.ChannelType(); t != "session" {
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("unknown channel type: %s", t))
		return
	}

	conn, requests, err := newChannel.Accept()

	if err != nil {
		klog.Errorf("error accepting channel, %v", err)
		return
	}

	term := term.NewTerminal(conn, "")
	term.SetPrompt(string(term.Escape.Red) + "> " + string(term.Escape.Reset))

	go func() {
		defer conn.Close()

		log.SetOutput(io.MultiWriter(os.Stdout, term))
		defer log.SetOutput(io.MultiWriter(os.Stdout))

		handler(term)

		klog.Infof("closing ssh connection")
	}()

	go func() {
		for req := range requests {
			reqSize := len(req.Payload)

			switch req.Type {
			case "shell":
				// do not accept payload commands
				if len(req.Payload) == 0 {
					req.Reply(true, nil)
				}
			case "pty-req":
				// p10, 6.2.  Requesting a Pseudo-Terminal, RFC4254
				if reqSize < 4 {
					klog.Errorf("malformed pty-req request")
					continue
				}

				termVariableSize := int(req.Payload[3])

				if reqSize < 4+termVariableSize+8 {
					klog.Errorf("malformed pty-req request")
					continue
				}

				w := binary.BigEndian.Uint32(req.Payload[4+termVariableSize:])
				h := binary.BigEndian.Uint32(req.Payload[4+termVariableSize+4:])

				term.SetSize(int(w), int(h))

				req.Reply(true, nil)
			case "window-change":
				// p10, 6.7.  Window Dimension Change Message, RFC4254
				if reqSize < 8 {
					klog.Errorf("malformed window-change request")
					continue
				}

				w := binary.BigEndian.Uint32(req.Payload)
				h := binary.BigEndian.Uint32(req.Payload[4:])

				term.SetSize(int(w), int(h))
			}
		}
	}()
}

func handleChannels(chans <-chan ssh.NewChannel, handler consoleHandler) {
	for newChannel := range chans {
		go handleChannel(newChannel, handler)
	}
}

func startSSHServer(ctx context.Context, listener net.Listener, addr string, port uint16, handler consoleHandler) {
	srv := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	if err != nil {
		log.Fatal("private key generation error: ", err)
	}

	signer, err := ssh.NewSignerFromKey(key)

	if err != nil {
		log.Fatal("key conversion error: ", err)
	}

	klog.Infof("TA starting ssh server (%s) at %s:%d", ssh.FingerprintSHA256(signer.PublicKey()), addr, port)

	srv.AddHostKey(signer)

	connsToClose := []*ssh.ServerConn{}
	defer func() {
		for _, sc := range connsToClose {
			klog.Infof("Closing SSH connection from %s", sc.RemoteAddr())
			sc.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			klog.Infof("SSH server exiting: %v", ctx.Err())
			return
		default:
		}
		conn, err := listener.Accept()

		if err != nil {
			klog.Errorf("error accepting connection, %v", err)
			continue
		}

		sshConn, chans, reqs, err := ssh.NewServerConn(conn, srv)

		if err != nil {
			klog.Errorf("error accepting handshake, %v", err)
			continue
		}
		connsToClose = append(connsToClose, sshConn)

		klog.Infof("new ssh connection from %s (%s)", sshConn.RemoteAddr(), sshConn.ClientVersion())

		go ssh.DiscardRequests(reqs)
		go handleChannels(chans, handler)
	}
}
