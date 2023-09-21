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

package doq

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

const (
	defaultDoQTimeout       = time.Second * 5
	dialTimeout             = time.Second * 3
	connectionLostThreshold = time.Second * 5
	handshakeTimeout        = time.Second * 3
)

var (
	doqAlpn = []string{"doq"}
)

type Upstream struct {
	t          *quic.Transport
	addr       string
	tlsConfig  *tls.Config
	quicConfig *quic.Config

	cm sync.Mutex
	lc *lazyConn
}

// tlsConfig cannot be nil, it should have Servername or InsecureSkipVerify.
func NewUpstream(addr string, lc *net.UDPConn, tlsConfig *tls.Config, quicConfig *quic.Config) (*Upstream, error) {
	srk, err := initSrk()
	if err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		return nil, errors.New("nil tls config")
	}

	tlsConfig = tlsConfig.Clone()
	tlsConfig.NextProtos = doqAlpn

	return &Upstream{
		t: &quic.Transport{
			Conn:              lc,
			StatelessResetKey: (*quic.StatelessResetKey)(srk),
		},
		addr:       addr,
		tlsConfig:  tlsConfig,
		quicConfig: quicConfig,
	}, nil
}

func initSrk() (*[32]byte, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (u *Upstream) Close() error {
	return u.t.Close()
}

func (u *Upstream) newStream(ctx context.Context) (quic.Stream, *lazyConn, error) {
	var lc *lazyConn
	u.cm.Lock()
	if u.lc == nil { // First dial.
		u.lc = u.asyncDialConn()
	} else {
		select {
		case <-u.lc.dialFinished:
			if u.lc.err != nil { // previous dial failed
				u.lc = u.asyncDialConn()
			} else {
				err := u.lc.c.Context().Err()
				if err != nil { // previous connection is dead or closed
					u.lc = u.asyncDialConn()
				}
				// previous connection looks good
			}
		default:
			// still dialing
		}
	}
	lc = u.lc
	u.cm.Unlock()

	select {
	case <-lc.dialFinished:
		if lc.c == nil {
			return nil, nil, lc.err
		}
		s, err := lc.c.OpenStream()
		if err != nil {
			lc.queryFailedAndCloseIfConnLost(time.Now())
			return nil, nil, err
		}
		return s, lc, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

type lazyConn struct {
	cancelDial func()

	m            sync.Mutex
	closed       bool
	dialFinished chan struct{}
	c            quic.Connection
	err          error

	latestRecvMs atomic.Int64
}

func (u *Upstream) asyncDialConn() *lazyConn {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	lc := &lazyConn{
		cancelDial:   cancel,
		dialFinished: make(chan struct{}),
	}

	go func() {
		defer cancel()

		ua, err := net.ResolveUDPAddr("udp", u.addr) // TODO: Support bootstrap.
		if err != nil {
			lc.err = err
			return
		}

		var c quic.Connection
		ec, err := u.t.DialEarly(ctx, ua, u.tlsConfig, u.quicConfig)
		if ec != nil {
			// This is a workaround to
			// 1. recover from strange 0rtt rejected err.
			// 2. avoid NextConnection might block forever.
			// TODO: Remove this workaround.
			select {
			case <-ctx.Done():
				err = context.Cause(ctx)
				ec.CloseWithError(0, "")
			case <-ec.HandshakeComplete():
				c = ec.NextConnection()
			}
		}

		var closeC bool
		lc.m.Lock()
		if lc.closed {
			closeC = true // lc was closed, nothing to do
		} else {
			if err != nil {
				lc.err = err
			} else {
				lc.c = c
				lc.saveLatestRespRecvTime(time.Now())
			}
			close(lc.dialFinished)
		}
		lc.m.Unlock()

		if closeC && c != nil { // lc was closed while dialing.
			c.CloseWithError(0, "")
		}
	}()
	return lc
}

func (lc *lazyConn) close() {
	lc.m.Lock()
	defer lc.m.Unlock()

	if lc.closed {
		return
	}
	lc.closed = true

	select {
	case <-lc.dialFinished:
		if lc.c != nil {
			lc.c.CloseWithError(0, "")
		}
	default:
		lc.cancelDial()
		lc.err = net.ErrClosed
		close(lc.dialFinished)
	}
}

func (lc *lazyConn) saveLatestRespRecvTime(t time.Time) {
	lc.latestRecvMs.Store(t.UnixMilli())
}

func (lc *lazyConn) latestRespRecvTime() time.Time {
	return time.UnixMilli(lc.latestRecvMs.Load())
}

func (lc *lazyConn) queryFailedAndCloseIfConnLost(t time.Time) {
	// No msg received for a quit long time. This connection may be lost.
	if time.Since(lc.latestRespRecvTime()) > connectionLostThreshold {
		lc.close()
	}
}

func (u *Upstream) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	payload, err := pool.PackTCPBuffer(q)
	if err != nil {
		return nil, err
	}
	// 4.2.1.  DNS Message IDs
	//    When sending queries over a QUIC connection, the DNS Message ID MUST
	//    be set to 0.  The stream mapping for DoQ allows for unambiguous
	//    correlation of queries and responses, so the Message ID field is not
	//    required.
	(*payload)[2], (*payload)[3] = 0, 0

	s, lc, err := u.newStream(ctx)
	if err != nil {
		return nil, err
	}

	type res struct {
		resp *dns.Msg
		err  error
	}
	rc := make(chan res, 1)
	go func() {
		defer pool.ReleaseBuf(payload)
		defer s.Close()
		r, err := u.exchange(s, *payload)
		rc <- res{resp: r, err: err}
		if err != nil {
			lc.queryFailedAndCloseIfConnLost(time.Now())
		} else {
			lc.saveLatestRespRecvTime(time.Now())
		}
	}()

	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case r := <-rc:
		resp := r.resp
		err := r.err
		if resp != nil {
			resp.Id = q.Id
		}
		return resp, err
	}
}

func (u *Upstream) exchange(s quic.Stream, payload []byte) (*dns.Msg, error) {
	s.SetDeadline(time.Now().Add(defaultDoQTimeout))

	_, err := s.Write(payload)
	if err != nil {
		return nil, err
	}

	resp, _, err := dnsutils.ReadMsgFromTCP(s)
	if err != nil {
		return nil, err
	}
	return resp, nil
}