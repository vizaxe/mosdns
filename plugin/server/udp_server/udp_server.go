/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package udp_server

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"net"
	"os"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/server"
	"github.com/IrineSistiana/mosdns/v5/plugin/server/server_utils"
)

const PluginType = "udp_server"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

type Args struct {
	Entry  string   `yaml:"entry"`
	Listen []string `yaml:"listen"`
}

func (a *Args) init() {
	if len(a.Listen) == 0 {
		a.Listen = []string{"127.0.0.1:53"}
	}
}

type UdpServer struct {
	args        *Args
	cs          []net.PacketConn
	listenPaths []string
}

func (s *UdpServer) Close() error {
	var err error
	for _, c := range s.cs {
		if e := c.Close(); e != nil {
			err = e
		}
	}
	for _, p := range s.listenPaths {
		if len(p) > 0 {
			os.Remove(p)
		}
	}
	return err
}

func Init(bp *coremain.BP, args any) (any, error) {
	return StartServer(bp, args.(*Args))
}

func StartServer(bp *coremain.BP, args *Args) (*UdpServer, error) {
	dh, err := server_utils.NewHandler(bp, args.Entry)
	if err != nil {
		return nil, fmt.Errorf("failed to init dns handler, %w", err)
	}

	var cs []net.PacketConn
	var listenPaths []string

	for _, listenAddr := range args.Listen {
		listenerNetwork := "udp"
		if strings.HasPrefix(listenAddr, "@") || strings.HasPrefix(listenAddr, "/") {
			listenerNetwork = "unixgram"
		}

		var listenPath string
		if listenerNetwork == "unixgram" && strings.HasPrefix(listenAddr, "/") {
			listenPath = listenAddr
			os.Remove(listenPath)
		}

		socketOpt := server_utils.ListenerSocketOpts{
			SO_REUSEPORT: true,
			SO_RCVBUF:    64 * 1024,
		}
		lc := net.ListenConfig{Control: server_utils.ListenerControl(socketOpt)}

		c, err := lc.ListenPacket(context.Background(), listenerNetwork, listenAddr)
		if err != nil {
			for _, cc := range cs {
				cc.Close()
			}
			return nil, fmt.Errorf("failed to create socket on %s, %w", listenAddr, err)
		}
		bp.L().Info("udp server started", zap.Stringer("addr", c.LocalAddr()))

		cs = append(cs, c)
		listenPaths = append(listenPaths, listenPath)

		go func(c net.PacketConn, network string) {
			defer c.Close()
			var serveErr error
			if network == "unixgram" {
				serveErr = server.ServeUnix(c.(*net.UnixConn), dh, server.UDPServerOpts{Logger: bp.L()})
			} else {
				serveErr = server.ServeUDP(c.(*net.UDPConn), dh, server.UDPServerOpts{Logger: bp.L()})
			}
			bp.M().GetSafeClose().SendCloseSignal(serveErr)
		}(c, listenerNetwork)
	}

	return &UdpServer{
		args:        args,
		cs:          cs,
		listenPaths: listenPaths,
	}, nil
}
