package lock

import (
	"runtime"
	"sync"
	"sync/atomic"
)

type spinlock uint32

const maxBackOff = 16

func NewSpinLock() sync.Locker {
	return new(spinlock)
}

func (s *spinlock) Lock() {
	backoff := 1
	for {
		if atomic.CompareAndSwapUint32((*uint32)(s), 0, 1) {
			break
		}

		for i := 0; i < backoff; i++ {
			runtime.Gosched()
		}

		if backoff < maxBackOff {
			backoff = backoff << 1
		}
	}

}

func (s *spinlock) Unlock() {
	atomic.StoreUint32((*uint32)(s), 0)
}
