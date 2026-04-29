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

package client_interface

import (
	"fmt"
	"net"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/IrineSistiana/mosdns/v5/plugin/matcher/base_ip"
)

const PluginType = "client_interface"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

type Args = base_ip.Args

func QuickSetup(bq sequence.BQ, s string) (sequence.Matcher, error) {
	ips := getClientInterfaceIps(s)
	cidrs := ipNetsToCIDRString(ips)
	// 	fmt.Println("网卡cidrs = " + cidrs)
	return base_ip.NewMatcher(bq, base_ip.ParseQuickSetupArgs(cidrs), matchClientAddr)
}

func getClientInterfaceIps(name string) []net.IPNet {

	// 	fmt.Println("网卡名称 = " + name)

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	ips := make([]net.IPNet, 0)

	for _, i := range interfaces {
		if i.Name != name {
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch addr.(type) {
			case *net.IPNet:
				ips = append(ips, *addr.(*net.IPNet)) // 解引用指针
			case *net.IPAddr:
				ips = append(ips, ipAddrToIPNet(*addr.(*net.IPAddr)))
			}
		}
	}

	return ips
}

func ipAddrToIPNet(ipAddr net.IPAddr) net.IPNet {
	var mask net.IPMask
	if ipAddr.IP.To4() != nil {
		mask = net.CIDRMask(32, 32) // IPv4使用/32掩码
	} else {
		mask = net.CIDRMask(128, 128) // IPv6使用/128掩码
	}
	return net.IPNet{
		IP:   ipAddr.IP,
		Mask: mask,
	}
}

// 单个IPNet转CIDR字符串
func ipNetToCIDRString(ipNet net.IPNet) string {
	ones, _ := ipNet.Mask.Size()
	return fmt.Sprintf("%s/%d", ipNet.IP.String(), ones)
}

// 多个IPNet转空格分隔的CIDR字符串
func ipNetsToCIDRString(ipNets []net.IPNet) string {
	var cidrs []string
	for _, ipNet := range ipNets {
		cidrs = append(cidrs, ipNetToCIDRString(ipNet))
	}
	return strings.Join(cidrs, " ")
}

func matchClientAddr(qCtx *query_context.Context, m netlist.Matcher) (bool, error) {
	addr := qCtx.ServerMeta.ClientAddr
	if !addr.IsValid() {
		return false, nil
	}

	// 	fmt.Println("客户端ip = %v", addr)
	// 	fmt.Println("是否匹配 = %v", m.Match(addr))
	return m.Match(addr), nil
}
