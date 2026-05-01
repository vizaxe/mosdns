/*
 * Copyright (C) 2024, Vizaxe
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

package network_interface

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"net"
	"net/netip"
	"sync"
)

const PluginType = "network_interface"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var mutex sync.Mutex
var pluginCache = make(map[string]*networkInterface)

var _ sequence.Executable = (*networkInterface)(nil)

type networkInterface struct {
	args *Args
}

type Args struct {
	InterfaceName string `yaml:"interface"`
}

func Init(bp *coremain.BP, args any) (any, error) {
	name := args.(*Args).InterfaceName
	return getNetworkInterfacePlugin(name), nil
}

func QuickSetup(_ sequence.BQ, name string) (any, error) {
	return getNetworkInterfacePlugin(name), nil
}

func getNetworkInterfacePlugin(name string) *networkInterface {
	mutex.Lock()
	plugin := pluginCache[name]
	if plugin == nil {
		plugin = &networkInterface{
			args: &Args{
				InterfaceName: name,
			},
		}
		pluginCache[name] = plugin
	}
	mutex.Unlock()
	return plugin
}

func (b *networkInterface) Exec(_ context.Context, qCtx *query_context.Context) error {
	if r := b.response(qCtx.Q()); r != nil {
		qCtx.SetResponse(r)
	}
	return nil
}

func (b *networkInterface) response(m *dns.Msg) *dns.Msg {
	if len(m.Question) != 1 {
		return nil
	}
	q := m.Question[0]
	typ := q.Qtype
	fqdn := q.Name
	if q.Qclass != dns.ClassINET || (typ != dns.TypeA && typ != dns.TypeAAAA) {
		return nil
	}

	name := b.args.InterfaceName

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	ipv4 := make([]netip.Addr, 0)
	ipv6 := make([]netip.Addr, 0)

	for _, i := range interfaces {
		if i.Name != name {
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			if ip == nil {
				continue
			}

			addr, ok := netip.AddrFromSlice(ip)
			if ok {
				if ip.To4() != nil {
					ipv4 = append(ipv4, addr)
				} else {
					ipv6 = append(ipv6, addr)
				}
			}
		}

		break
	}

	if len(ipv4)+len(ipv6) == 0 {
		return nil // no such host
	}

	r := new(dns.Msg)
	r.SetReply(m)
	switch {
	case typ == dns.TypeA && len(ipv4) > 0:
		for _, ip := range ipv4 {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   fqdn,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    10,
				},
				A: ip.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
	case typ == dns.TypeAAAA && len(ipv6) > 0:
		for _, ip := range ipv6 {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   fqdn,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    10,
				},
				AAAA: ip.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
	}

	// Append fake SOA record for empty reply.
	if len(r.Answer) == 0 {
		r.Ns = []dns.RR{dnsutils.FakeSOA(fqdn)}
	}
	return r
}
