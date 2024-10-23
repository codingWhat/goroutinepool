package goroutinepool

import (
	"fmt"
	"log"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type Pool interface {
	Invoke(any)
	Release()
	Running() int
}

var workerChanCap = func() int {
	// Use blocking workerChan if GOMAXPROCS=1.
	// This immediately switches Serve to WorkerFunc, which results
	// in higher performance (under go1.5 at least).
	if runtime.GOMAXPROCS(0) == 1 {
		return 0
	}

	// Use non-blocking workerChan if GOMAXPROCS>1,
	// since otherwise the Serve caller (Acceptor) may lag accepting
	// new connections if WorkerFunc is CPU-bound.
	return 1
}()

type TaskFunc func(any) error

type worker struct {
	lastUseTime time.Time
	ch          chan any
}

type poolManager struct {
	fn                TaskFunc
	pool              sync.Pool
	workerMaxIdleTime time.Duration
	ready             []*worker
	mu                sync.RWMutex

	isStopped atomic.Bool
	stopSig   chan struct{}

	curWorkers int
	maxWorkers int
}

func NewWithFunc(size int, tf TaskFunc, options ...Option) Pool {
	p := &poolManager{
		pool:              sync.Pool{New: func() any { return &worker{ch: make(chan any, workerChanCap)} }},
		maxWorkers:        size,
		fn:                tf,
		workerMaxIdleTime: 1 * time.Minute,
		stopSig:           make(chan struct{}),
		ready:             make([]*worker, 0, size),
	}

	for _, op := range options {
		op(p)
	}

	go WithRecovery(p.start)

	return p
}
func (p *poolManager) Running() int {
	p.mu.Lock()
	num := len(p.ready)
	p.mu.Unlock()
	return num
}
func (p *poolManager) runWorker(w *worker) {
	for v := range w.ch {
		if v == nil {
			break
		}

		err := p.fn(v)
		if err != nil {
			log.Printf("worker executing happened err:%+v", err)
		}

		if p.isStopped.Load() {
			break
		}
		p.toReady(w)
	}
	p.mu.Lock()
	p.curWorkers--
	curWorkers := p.curWorkers
	p.mu.Unlock()

	//放入池子
	if curWorkers >= p.maxWorkers {
		return
	}
	p.pool.Put(w)
}

func (p *poolManager) toReady(w *worker) {
	w.lastUseTime = time.Now()
	p.mu.Lock()
	p.ready = append(p.ready, w)
	p.mu.Unlock()
}

func (p *poolManager) getWorker() *worker {

	p.mu.Lock()
	available := len(p.ready)
	p.mu.Unlock()
	var w *worker
	if available == 0 {
		//说明用完了，重新创建worker
		p.mu.Lock()
		if p.curWorkers < p.maxWorkers {
			w = p.pool.Get().(*worker)
			p.curWorkers++
			p.mu.Unlock()
			go func() {
				p.runWorker(w)
			}()
			return w
		} else {
			p.mu.Unlock()
			//上层重试
			return nil
		}

	}

	p.mu.Lock()
	w = p.ready[0]
	p.ready[0] = nil
	p.ready = p.ready[1:]
	p.mu.Unlock()
	return w
}

func (p *poolManager) start() {
	var scratch []*worker
	for {
		p.clean(&scratch)
		select {
		case <-p.stopSig:
			return
		default:
			time.Sleep(p.workerMaxIdleTime)
		}
	}
}

func WithRecovery(fn func()) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("[WithRecovery] err:", err, string(debug.Stack()))
		}
	}()

	fn()
}

// 内存优化：
// 1.注意传的是切片指针，避免了切片头信息的复制
// 2.复用切片，不需要每次都申请内存空间
func (p *poolManager) clean(scratch *[]*worker) {
	cleanTime := time.Now().Add(-p.workerMaxIdleTime)

	p.mu.Lock()
	n := len(p.ready)
	if n == 0 {
		p.mu.Unlock()
		return
	}
	lo := 0
	hi := n - 1
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if p.ready[mid].lastUseTime.After(cleanTime) {
			hi = mid - 1
		} else {
			lo = mid
		}
	}

	if p.ready[lo].lastUseTime.After(cleanTime) {
		p.mu.Unlock()
		return
	}

	//更新原切片，包括头信息中的长度
	*scratch = append((*scratch)[:0], p.ready[0:hi+1]...)
	m := copy(p.ready, p.ready[hi+1:])
	for i := m; i < n; i++ {
		p.ready[i] = nil
	}
	p.ready = p.ready[:m]
	p.mu.Unlock()

	tmp := *scratch
	//注意这里索引访问，又避免了内存拷贝
	for i := range tmp {
		tmp[i].ch <- nil
	}
}

func (p *poolManager) Invoke(a any) {
	w := p.getWorker()
	for w == nil {
		w = p.getWorker()
		runtime.Gosched() //防止无限循环
	}
	w.ch <- a
}

func (p *poolManager) Release() {
	if p.isStopped.Load() {
		return
	}
	p.isStopped.Store(true)
	close(p.stopSig)
	for i := range p.ready {
		p.ready[i].ch <- nil
	}

	p.ready = nil
	p.curWorkers = 0
}
