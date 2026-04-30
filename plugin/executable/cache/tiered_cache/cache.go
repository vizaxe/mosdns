package tiered_cache

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const PluginType = "tiered_cache"

const defaultAsyncUpdateTimeout = time.Second * 5
const defaultRetryCount = 1

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.RecursiveExecutable = (*TieredCache)(nil)

type Args struct {
	L1Tag      string `yaml:"l1_tag"`
	L2Tag      string `yaml:"l2_tag"`
	RetryCount int    `yaml:"retry_count"`
}

type dnsCacher interface {
	QueryDns(q *dns.Msg) (*dns.Msg, bool)
	StoreDns(q *dns.Msg, r *dns.Msg)
}

type TieredCache struct {
	l1   dnsCacher
	l2   dnsCacher
	bp   *coremain.BP
	args *Args

	lazyUpdateSF singleflight.Group
}

func Init(bp *coremain.BP, args any) (any, error) {
	a := args.(*Args)
	return NewTieredCache(bp, a)
}

func NewTieredCache(bp *coremain.BP, args *Args) (*TieredCache, error) {
	if len(args.L1Tag) == 0 {
		return nil, fmt.Errorf("l1_tag is required")
	}
	if len(args.L2Tag) == 0 {
		return nil, fmt.Errorf("l2_tag is required")
	}
	if args.RetryCount <= 0 {
		args.RetryCount = defaultRetryCount
	}

	p1 := bp.M().GetPlugin(args.L1Tag)
	if p1 == nil {
		return nil, fmt.Errorf("l1 cache plugin [%s] not found", args.L1Tag)
	}
	l1, ok := p1.(dnsCacher)
	if !ok {
		return nil, fmt.Errorf("plugin [%s] does not implement cache interface", args.L1Tag)
	}

	p2 := bp.M().GetPlugin(args.L2Tag)
	if p2 == nil {
		return nil, fmt.Errorf("l2 cache plugin [%s] not found", args.L2Tag)
	}
	l2, ok := p2.(dnsCacher)
	if !ok {
		return nil, fmt.Errorf("plugin [%s] does not implement cache interface", args.L2Tag)
	}

	return &TieredCache{
		l1:   l1,
		l2:   l2,
		bp:   bp,
		args: args,
	}, nil
}

func (t *TieredCache) queryKey(q *dns.Msg) string {
	if len(q.Question) == 0 {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d", strings.ToLower(q.Question[0].Name), q.Question[0].Qtype, q.Question[0].Qclass)
}

func (t *TieredCache) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	if qCtx.GetBlackHoleTag() != "" {
		return next.ExecNext(ctx, qCtx)
	}

	q := qCtx.Q()
	qCtx.CacheQueried = true

	// try L1
	if r, lazyHit := t.l1.QueryDns(q); r != nil {
		r.Id = q.Id
		qCtx.SetResponse(r)
		qCtx.CacheHit = true
		qCtx.CacheName = t.bp.Tag() + " -> " + t.args.L1Tag
		if lazyHit {
			t.asyncUpdate(q, next)
		}
		err := next.ExecNext(ctx, qCtx)
		if qCtx.GetBlackHoleTag() == "" {
			query_context.RecordCache(true)
		}
		return err
	}

	// try L2
	if r, lazyHit := t.l2.QueryDns(q); r != nil {
		r.Id = q.Id
		qCtx.SetResponse(r)
		t.l1.StoreDns(q, r)
		qCtx.CacheHit = true
		qCtx.CacheName = t.bp.Tag() + " -> " + t.args.L2Tag
		if lazyHit {
			t.asyncUpdate(q, next)
		}
		err := next.ExecNext(ctx, qCtx)
		if qCtx.GetBlackHoleTag() == "" {
			query_context.RecordCache(true)
		}
		return err
	}

	err := next.ExecNext(ctx, qCtx)

	if qCtx.GetBlackHoleTag() == "" {
		qCtx.CacheHit = false
		query_context.RecordCache(false)
		if qCtx.R() != nil {
			t.l1.StoreDns(q, qCtx.R())
			t.l2.StoreDns(q, qCtx.R())
		}
	}

	return err
}

func (t *TieredCache) asyncUpdate(q *dns.Msg, next sequence.ChainWalker) {
	key := t.queryKey(q)
	if key == "" {
		return
	}

	qCopy := q.Copy()
	lazyUpdateFunc := func() (any, error) {
		defer t.lazyUpdateSF.Forget(key)

		ctx, cancel := context.WithTimeout(context.Background(), defaultAsyncUpdateTimeout)
		defer cancel()

		var lastErr error
		for i := 0; i <= t.args.RetryCount; i++ {
			if i > 0 {
				time.Sleep(time.Duration(100*(1<<uint(i-1))) * time.Millisecond)
			}

			retryNext := next
			qCtx := query_context.NewContext(qCopy)
			err := retryNext.ExecNext(ctx, qCtx)
			if err != nil {
				lastErr = err
				t.bp.L().Warn("tiered_cache 异步更新失败",
					zap.String("query", q.Question[0].String()),
					zap.Int("attempt", i+1),
					zap.Error(err),
				)
				continue
			}
			if r := qCtx.R(); r != nil {
				t.l1.StoreDns(qCopy, r)
				t.l2.StoreDns(qCopy, r)
				lastErr = nil
				break
			}
		}

		if lastErr != nil {
			t.bp.L().Error("tiered_cache 异步更新重试耗尽",
				zap.String("query", q.Question[0].String()),
				zap.Int("retry_count", t.args.RetryCount),
				zap.Error(lastErr),
			)
		}
		return nil, lastErr
	}

	t.lazyUpdateSF.DoChan(key, lazyUpdateFunc)
}
