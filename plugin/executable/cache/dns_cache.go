package cache

import (
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/miekg/dns"
	"time"
)

func SetDefaultVal(m *dns.Msg) *dns.Msg {
	if m == nil {
		return nil
	}
	if m.Answer == nil {
		m.Answer = make([]dns.RR, 0)
	}
	if m.Ns == nil {
		m.Ns = make([]dns.RR, 0)
	}
	if m.Extra == nil {
		m.Extra = make([]dns.RR, 0)
	}
	return m
}

// CopyMsgNoOpt deep copies m and excludes OPT records.
func CopyMsgNoOpt(m *dns.Msg) *dns.Msg {
	if m == nil {
		return nil
	}

	m2 := new(dns.Msg)
	m2.MsgHdr = m.MsgHdr
	m2.Compress = m.Compress

	if len(m.Question) > 0 {
		m2.Question = make([]dns.Question, len(m.Question))
		copy(m2.Question, m.Question)
	}

	lenExtra := len(m.Extra)
	for _, r := range m.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			lenExtra--
		}
	}

	s := make([]dns.RR, len(m.Answer)+len(m.Ns)+lenExtra)
	m2.Answer, s = s[:0:len(m.Answer)], s[len(m.Answer):]
	m2.Ns, s = s[:0:len(m.Ns)], s[len(m.Ns):]
	m2.Extra = s[:0:lenExtra]

	for _, r := range m.Answer {
		m2.Answer = append(m2.Answer, dns.Copy(r))
	}
	for _, r := range m.Ns {
		m2.Ns = append(m2.Ns, dns.Copy(r))
	}
	for _, r := range m.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			continue
		}
		m2.Extra = append(m2.Extra, dns.Copy(r))
	}
	return m2
}

func CalculateMsgTTL(r *dns.Msg) (msgTtl time.Duration, shouldCache bool) {
	if r.Truncated {
		return 0, false
	}

	switch r.Rcode {
	case dns.RcodeNameError:
		msgTtl = 30 * time.Second
	case dns.RcodeServerFailure:
		msgTtl = 5 * time.Second
	case dns.RcodeSuccess:
		minTTL := dnsutils.GetMinimalTTL(r)
		if len(r.Answer) == 0 {
			const maxEmptyAnswerTtl = 300
			msgTtl = time.Duration(min(minTTL, maxEmptyAnswerTtl)) * time.Second
		} else {
			msgTtl = time.Duration(minTTL) * time.Second
		}
	default:
		return 0, false
	}
	if msgTtl <= 0 {
		return 0, false
	}
	return msgTtl, true
}

// PrepareCachedResponse adjusts TTL based on elapsed time.
// Returns the response and whether it's a lazy (expired) hit.
func PrepareCachedResponse(item *Item, lazyCacheEnabled bool, lazyTtl int) (*dns.Msg, bool) {
	now := time.Now()

	if now.Before(item.ExpirationTime) {
		r := CopyMsgNoOpt(item.Resp)
		dnsutils.SubtractTTL(r, uint32(now.Sub(item.StoredTime).Seconds()))
		return r, false
	}

	if lazyCacheEnabled {
		r := CopyMsgNoOpt(item.Resp)
		dnsutils.SetTTL(r, uint32(lazyTtl))
		return r, true
	}

	return nil, false
}

func NewCacheItem(r *dns.Msg, msgTtl time.Duration, blackHoleTag string) *Item {
	now := time.Now()
	return &Item{
		Resp:           CopyMsgNoOpt(r),
		StoredTime:     now,
		ExpirationTime: now.Add(msgTtl),
		BlockHoleTag:   blackHoleTag,
	}
}
