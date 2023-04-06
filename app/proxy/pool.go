package proxy

import (
	"sync"
)

type bytesPool struct{ pool sync.Pool }

const sz = 32 * 1024

func newPool() *bytesPool {
	return &bytesPool{
		pool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, sz)
				return &b
			},
		},
	}
}

func (p *bytesPool) Get() *[]byte {
	return p.pool.Get().(*[]byte)
}

func (p *bytesPool) Put(b *[]byte) {
	p.pool.Put(b)
}
