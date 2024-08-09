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

package redis_cache

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/redis_cache"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/miekg/dns"
	"golang.org/x/exp/constraints"
)

// getMsgKey returns a string key for the query msg, or an empty
// string if query should not be cached.
func getMsgKey(q *dns.Msg, separator string) string {
	question := q.Question[0]
	return fmt.Sprintf("%s%s%s%s%s", dns.TypeToString[question.Qtype], separator, dns.ClassToString[question.Qclass], separator, question.Name)
}

func copyNoOpt(m *dns.Msg) *dns.Msg {
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

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// getRespFromCache returns the cached response from cache.
// The ttl of returned msg will be changed properly.
// Returned bool indicates whether this response is hit by lazy cache.
// Note: Caller SHOULD change the msg id because it's not same as query's.
func getRespFromCache(key string, backend *redis_cache.Cache, lazyCacheEnabled bool, lazyTtl int) (*dns.Msg, bool) {
	// Lookup cache
	v, ok := backend.Get(key)

	// Cache hit
	if ok && v != nil {
		now := time.Now()

		expirationTime := v.ExpirationTime
		storedTime := v.StoredTime
		resp := v.Resp
		// Not expired.
		if now.Before(expirationTime) {
			r := resp.Copy()
			dnsutils.SubtractTTL(r, uint32(now.Sub(storedTime).Seconds()))
			return r, false
		}

		// Msg expired but cache isn't. This is a lazy cache enabled entry.
		// If lazy cache is enabled, return the response.
		if lazyCacheEnabled {
			r := resp.Copy()
			dnsutils.SetTTL(r, uint32(lazyTtl))
			return r, true
		}
	}

	// cache miss
	return nil, false
}

// saveRespToCache saves r to cache backend. It returns false if r
// should not be cached and was skipped.
func saveRespToCache(msgKey string, r *dns.Msg, backend *redis_cache.Cache, lazyCacheTtl int) bool {
	if r.Truncated != false {
		return false
	}

	var msgTtl time.Duration
	var cacheTtl time.Duration
	switch r.Rcode {
	case dns.RcodeNameError:
		msgTtl = time.Second * 30
		cacheTtl = msgTtl
	case dns.RcodeServerFailure:
		msgTtl = time.Second * 5
		cacheTtl = msgTtl
	case dns.RcodeSuccess:
		minTTL := dnsutils.GetMinimalTTL(r)
		if len(r.Answer) == 0 { // Empty answer. Set ttl between 0~300.
			const maxEmtpyAnswerTtl = 300
			msgTtl = time.Duration(min(minTTL, maxEmtpyAnswerTtl)) * time.Second
			cacheTtl = msgTtl
		} else {
			msgTtl = time.Duration(minTTL) * time.Second
			if lazyCacheTtl > 0 {
				cacheTtl = time.Duration(lazyCacheTtl) * time.Second
			} else {
				cacheTtl = msgTtl
			}
		}
	}
	if msgTtl <= 0 || cacheTtl <= 0 {
		return false
	}

	now := time.Now()
	expirationTime := now.Add(msgTtl)
	v := &redis_cache.Item{
		Resp:           copyNoOpt(r),
		StoredTime:     now,
		ExpirationTime: expirationTime,
	}
	backend.Store(msgKey, v, cacheTtl)
	return true
}
