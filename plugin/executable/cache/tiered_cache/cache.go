package tiered_cache

import (
	"context"
	"fmt"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "tiered_cache"

const defaultAsyncUpdateTimeout = time.Second * 5

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.RecursiveExecutable = (*TieredCache)(nil)

type Args struct {
	L1Tag string `yaml:"l1_tag"`
	L2Tag string `yaml:"l2_tag"`
}

type dnsCacher interface {
	QueryDns(q *dns.Msg) (*dns.Msg, bool)
	StoreDns(q *dns.Msg, r *dns.Msg)
}

type TieredCache struct {
	l1 dnsCacher
	l2 dnsCacher
	bp *coremain.BP
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
		l1: l1,
		l2: l2,
		bp: bp,
	}, nil
}

func (t *TieredCache) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	q := qCtx.Q()

	// try L1
	var r *dns.Msg = nil
	var lazyHit = false
	r, lazyHit = t.l1.QueryDns(q)
	if r != nil {
		r.Id = q.Id
		qCtx.SetResponse(r)
		qCtx.CacheHit = true
		qCtx.CacheName = "tiered_cache L1"
		query_context.RecordCache(true)
	}

	// try L2
	if r == nil {
		r, lazyHit = t.l2.QueryDns(q)
		if r != nil {
			r.Id = q.Id
			qCtx.SetResponse(r)
			t.l1.StoreDns(q, r)
			qCtx.CacheHit = true
			qCtx.CacheName = "tiered_cache L2"
			query_context.RecordCache(true)
			if lazyHit {
				go t.asyncUpdate(ctx, q, next)
			}
		}
	}

	err := next.ExecNext(ctx, qCtx)

	// both miss
	if r == nil {
		r = qCtx.R()
		qCtx.CacheHit = false
		query_context.RecordCache(false)
		if r != nil {
			t.l1.StoreDns(q, r)
			t.l2.StoreDns(q, r)
		}
	}
	return err
}

func (t *TieredCache) asyncUpdate(ctx context.Context, q *dns.Msg, next sequence.ChainWalker) {
	qCtx := query_context.NewContext(q.Copy())
	err := next.ExecNext(ctx, qCtx)
	if err != nil {
		return
	}
	if r := qCtx.R(); r != nil {
		t.l1.StoreDns(q, r)
		t.l2.StoreDns(q, r)
	}
}
