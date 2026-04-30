package pool

import (
	"math/bits"
	"sync"
)

var (
	GetBuf     func(size int) *[]byte
	ReleaseBuf func(b *[]byte)
)

func init() {
	p := newBufPool(20)
	GetBuf = p.get
	ReleaseBuf = p.release
}

type bufPool struct {
	ps []sync.Pool
}

func newBufPool(bitLen int) *bufPool {
	p := &bufPool{ps: make([]sync.Pool, bitLen+1)}
	for i := range p.ps {
		sz := (1 << i) - 1
		p.ps[i] = sync.Pool{New: func() any { b := make([]byte, sz); return &b }}
	}
	return p
}

func (p *bufPool) get(size int) *[]byte {
	idx := poolIndex(size)
	if idx >= len(p.ps) {
		b := make([]byte, size)
		return &b
	}
	b := p.ps[idx].Get().(*[]byte)
	*b = (*b)[:size]
	return b
}

func (p *bufPool) release(b *[]byte) {
	c := cap(*b)
	idx := poolIndex(c)
	if idx >= len(p.ps) {
		return
	}
	if c != (1<<idx)-1 {
		return
	}
	*b = (*b)[:c]
	p.ps[idx].Put(b)
}

func poolIndex(size int) int {
	if size <= 0 {
		return 0
	}
	x := uint64(size + 1)
	l := bits.Len64(x)
	if x == 1<<(l-1) {
		return l - 1
	}
	return l
}
