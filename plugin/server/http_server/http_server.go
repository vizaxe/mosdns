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

package http_server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/server"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/server/server_utils"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
)

const PluginType = "http_server"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

type Args struct {
	Entries []struct {
		Exec string `yaml:"exec"`
		Path string `yaml:"path"`
	} `yaml:"entries"`
	Listen      []string `yaml:"listen"`
	SrcIPHeader string   `yaml:"src_ip_header"`
	Cert        string   `yaml:"cert"`
	Key         string   `yaml:"key"`
	IdleTimeout int      `yaml:"idle_timeout"`
}

func (a *Args) init() {
	utils.SetDefaultNum(&a.IdleTimeout, 30)
}

type HttpServer struct {
	args        *Args
	server      *http.Server
	listenPaths []string
}

func (s *HttpServer) Close() error {
	err := s.server.Close()
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

func StartServer(bp *coremain.BP, args *Args) (*HttpServer, error) {
	mux := http.NewServeMux()
	for _, entry := range args.Entries {
		dh, err := server_utils.NewHandler(bp, entry.Exec)
		if err != nil {
			return nil, fmt.Errorf("failed to init dns handler, %w", err)
		}
		hhOpts := server.HttpHandlerOpts{
			GetSrcIPFromHeader: args.SrcIPHeader,
			Logger:             bp.L(),
		}
		hh := server.NewHttpHandler(dh, hhOpts)
		mux.Handle(entry.Path, hh)
	}

	socketOpt := server_utils.ListenerSocketOpts{
		SO_REUSEPORT: true,
		SO_RCVBUF:    64 * 1024,
	}
	lc := net.ListenConfig{Control: server_utils.ListenerControl(socketOpt)}

	var ls []net.Listener
	var listenPaths []string

	for _, listenAddr := range args.Listen {
		listenerNetwork := "tcp"
		if strings.HasPrefix(listenAddr, "@") || strings.HasPrefix(listenAddr, "/") {
			listenerNetwork = "unix"
		}

		var listenPath string
		if listenerNetwork == "unix" && strings.HasPrefix(listenAddr, "/") {
			listenPath = listenAddr
			os.Remove(listenPath)
		}

		l, err := lc.Listen(context.Background(), listenerNetwork, listenAddr)
		if err != nil {
			for _, ll := range ls {
				ll.Close()
			}
			return nil, fmt.Errorf("failed to listen socket on %s, %w", listenAddr, err)
		}
		bp.L().Info("http server started", zap.Stringer("addr", l.Addr()))

		ls = append(ls, l)
		listenPaths = append(listenPaths, listenPath)
	}

	hs := &http.Server{
		Handler:        mux,
		ReadTimeout:    time.Second,
		IdleTimeout:    time.Duration(args.IdleTimeout) * time.Second,
		MaxHeaderBytes: 512,
	}
	if err := http2.ConfigureServer(hs, &http2.Server{
		MaxReadFrameSize:             16 * 1024,
		IdleTimeout:                  time.Duration(args.IdleTimeout) * time.Second,
		MaxUploadBufferPerConnection: 65535,
		MaxUploadBufferPerStream:     65535,
	}); err != nil {
		for _, ll := range ls {
			ll.Close()
		}
		return nil, fmt.Errorf("failed to setup http2 server, %w", err)
	}

	for _, l := range ls {
		go func(l net.Listener) {
			var err error
			if len(args.Key)+len(args.Cert) > 0 {
				err = hs.ServeTLS(l, args.Cert, args.Key)
			} else {
				err = hs.Serve(l)
			}
			bp.M().GetSafeClose().SendCloseSignal(err)
		}(l)
	}
	return &HttpServer{
		args:        args,
		server:      hs,
		listenPaths: listenPaths,
	}, nil
}
