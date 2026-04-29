package server

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"os"
	"testing"
)

func TestIsPacketConn(t *testing.T) {
	// UDP
	//c, err := net.Dial("unixgram", "/tmp/udp.sock")
	//if err != nil {
	//	t.Fatalf("failed to dial: %v", err)
	//}
	//defer c.Close()
	//if !isPacketConn(c) {
	//	t.Error("Unix datagram connection should be a packet conn")
	//}
	//if !isPacketConn(struct{ *net.UnixConn }{c.(*net.UnixConn)}) {
	//	t.Error("Unix datagram connection (wrapped type) should be a packet conn")
	//}

	query("openresty.")
	query("www.qq.com.")
}

func query(str string) {
	// 构建DNS请求
	m := new(dns.Msg)
	m.SetQuestion(str, dns.TypeA)
	m.RecursionDesired = true
	msg, _ := m.Pack()

	fmt.Printf("query: %v\n", m)

	localSock, err := os.CreateTemp("", "mosdns-unixgram-request.sock")
	if err != nil {
		panic(err)
	}
	filePath := localSock.Name()
	os.Remove(filePath)

	c, err := net.DialUnix("unixgram",
		&net.UnixAddr{Name: filePath, Net: "unixgram"},
		&net.UnixAddr{Name: "/home/rice/project/github/mosdns/udp.sock", Net: "unixgram"})
	if err != nil {
		panic(err)
	}
	defer c.Close()

	if _, err := c.Write(msg); err != nil {
		panic(err)
	}

	// 读取DNS响应
	in := make([]byte, 512)
	_, err = c.Read(in)
	if err != nil {
		panic(err)
	}

	resp := new(dns.Msg)
	if err := resp.Unpack(in); err != nil {
		panic(err)
	}

	//i := &redis_cache.Item{
	//	Resp: resp,
	//}
	//r := redis_cache.Test(*i)

	fmt.Printf("DNS Response: %v\n", resp)
}
