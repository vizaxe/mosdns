package memory_cache

import (
	"github.com/IrineSistiana/mosdns/v5/pkg/cache_backend/memory_cache_backend"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/cache"
	"github.com/miekg/dns"
	"time"
)

// getRespFromCache returns the cached response from cache.
// The ttl of returned msg will be changed properly.
// Returned bool indicates whether this response is hit by lazy cache.
// Note: Caller SHOULD change the msg id because it's not same as query's.
func getRespFromCache(msgKey string, backend *memory_cache_backend.MemoryCache[key, *cache.Item], lazyCacheEnabled bool, lazyTtl int) (*dns.Msg, bool) {
	v, _, _ := backend.Get(key(msgKey))
	if v == nil {
		return nil, false
	}
	return cache.PrepareCachedResponse(v, lazyCacheEnabled, lazyTtl)
}

// saveRespToCache saves r to cache backend. It returns false if r
// should not be cached and was skipped.
func saveRespToCache(msgKey string, r *dns.Msg, backend *memory_cache_backend.MemoryCache[key, *cache.Item], lazyCacheTtl int) bool {
	msgTtl, ok := cache.CalculateMsgTTL(r, lazyCacheTtl)
	if !ok {
		return false
	}

	cacheTtl := msgTtl
	if lazyCacheTtl > 0 {
		cacheTtl = time.Duration(lazyCacheTtl) * time.Second
	}

	v := cache.NewCacheItem(r, msgTtl, "")
	backend.Store(key(msgKey), v, cacheTtl)
	return true
}
