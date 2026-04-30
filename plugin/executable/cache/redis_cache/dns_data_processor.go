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
	"context"
	"encoding/json"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/cache_backend"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/cache"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func (c *RedisCache) doLazyUpdate(msgKey string, qCtx *query_context.Context, next sequence.ChainWalker) {
	qCtxCopy := qCtx.Copy()
	lazyUpdateFunc := func() (any, error) {
		defer c.lazyUpdateSF.Forget(msgKey)
		qCtx := qCtxCopy

		c.logger.Debug("start lazy cache update", qCtx.InfoField())
		ctx, cancel := context.WithTimeout(context.Background(), cache_backend.DefaultLazyUpdateTimeout)
		defer cancel()

		err := next.ExecNext(ctx, qCtx)
		if err != nil {
			c.logger.Warn("failed to update lazy cache", qCtx.InfoField(), zap.Error(err))
		}

		r := qCtx.R()
		if r != nil {
			c.saveRespToCache(msgKey, r, c.args.LazyCacheTTL, qCtx.GetBlackHoleTag())
			c.updatedKey.Add(1)
		}
		c.logger.Debug("lazy cache updated", qCtx.InfoField())
		return nil, nil
	}
	c.lazyUpdateSF.DoChan(msgKey, lazyUpdateFunc)
}

func (c *RedisCache) saveRespToCache(msgKey string, r *dns.Msg, lazyCacheTtl int, blackHoleTag string) bool {
	msgTtl, ok := cache.CalculateMsgTTL(r)
	if !ok {
		return false
	}

	var cacheTtl time.Duration
	if lazyCacheTtl == redis.KeepTTL {
		cacheTtl = redis.KeepTTL
	} else if lazyCacheTtl > 0 {
		cacheTtl = time.Duration(lazyCacheTtl) * time.Second
	} else {
		cacheTtl = msgTtl
	}

	cache.SetDefaultVal(r)
	v := cache.NewCacheItem(r, msgTtl, blackHoleTag)
	msg, _ := json.Marshal(v)
	c.backend.Store(cache_backend.StringKey(msgKey), string(msg), cacheTtl)
	return true
}

func (c *RedisCache) getRespFromCache(msgKey string, lazyCacheEnabled bool, lazyTtl int) (*dns.Msg, bool) {
	v, _, ok := c.backend.Get(cache_backend.StringKey(msgKey))
	if !ok {
		return nil, false
	}
	item := UnmarshalDNSItem([]byte(v))
	if &item == nil {
		return nil, false
	}
	return cache.PrepareCachedResponse(&item, lazyCacheEnabled, lazyTtl)
}
