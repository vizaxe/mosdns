package redis_cache

import (
	"encoding/base64"
	"encoding/json"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/cache"
	"github.com/miekg/dns"
	"time"
)

const layout = "2006-01-02T15:04:05.999999999Z07:00"

type cacheItemJSON struct {
	Resp           string `json:"resp"`
	BlockHoleTag   string `json:"block_hole_tag"`
	StoredTime     string `json:"stored_time"`
	ExpirationTime string `json:"expiration_time"`
}

func marshalItem(item *cache.Item) string {
	v := &cacheItemJSON{
		Resp:           base64.StdEncoding.EncodeToString(dnsMsgToBytes(item.Resp)),
		BlockHoleTag:   item.BlockHoleTag,
		StoredTime:     item.StoredTime.Format(layout),
		ExpirationTime: item.ExpirationTime.Format(layout),
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func unmarshalItem(raw string) *cache.Item {
	if len(raw) == 0 {
		return nil
	}

	v := new(cacheItemJSON)
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return nil
	}

	storedTime, err := time.Parse(layout, v.StoredTime)
	if err != nil {
		return nil
	}
	expirationTime, err := time.Parse(layout, v.ExpirationTime)
	if err != nil {
		return nil
	}

	resp := bytesToDNSMsg(v.Resp)
	if resp == nil {
		return nil
	}

	return &cache.Item{
		Resp:           cache.SetDefaultVal(resp),
		StoredTime:     storedTime,
		ExpirationTime: expirationTime,
		BlockHoleTag:   v.BlockHoleTag,
	}
}

func dnsMsgToBytes(m *dns.Msg) []byte {
	b, _ := m.Pack()
	return b
}

func bytesToDNSMsg(data string) *dns.Msg {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil
	}
	m := new(dns.Msg)
	if err := m.Unpack(b); err != nil {
		return nil
	}
	return m
}
