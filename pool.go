package goroutinepool

import (
	"fmt"
	"github.com/codingWhat/goroutinepool/lock"
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
	blocking          int32
	running           int32
	mu                sync.Locker
	cond              *sync.Cond

	isStopped atomic.Bool
	stopSig   chan struct{}

	maxWorkers int
}

func NewWithFunc(size int, tf TaskFunc, options ...Option) Pool {
	p := &poolManager{
		pool:              sync.Pool{New: func() any { return &worker{ch: make(chan any, workerChanCap)} }},
		maxWorkers:        size,
		fn:                tf,
		workerMaxIdleTime: 1 * time.Minute,
		stopSig:           make(chan struct{}),
		mu:                lock.NewSpinLock(),
	}

	for _, op := range options {
		op(p)
	}

	p.cond = sync.NewCond(p.mu)

	go WithRecovery(p.start)

	return p
}

func (p *poolManager) Running() int {
	return int(atomic.LoadInt32(&p.running))
}
func (p *poolManager) incRunning() {
	atomic.AddInt32(&p.running, 1)
}

// decRunning decreases the number of the currently running goroutines.
func (p *poolManager) decRunning() {
	atomic.AddInt32(&p.running, -1)
}

func (p *poolManager) runWorker(w *worker) {

	p.incRunning()
	defer func() {
		p.decRunning()
	}()

	for v := range w.ch {
		if v == nil {
			break
		}
		if p.isStopped.Load() {
			break
		}

		err := p.fn(v)
		if err != nil {
			log.Printf("worker executing happened err:%+v", err)
		}
		p.toReady(w)
	}

	//放入池子
	if p.Running() >= p.maxWorkers {
		return
	}
	p.pool.Put(w)
}

func (p *poolManager) toReady(w *worker) {
	w.lastUseTime = time.Now()
	p.mu.Lock()
	p.ready = append(p.ready, w)
	p.cond.Signal()
	p.mu.Unlock()
}

func (p *poolManager) getWorker() *worker {

	p.mu.Lock()
	n := len(p.ready) - 1
	var w *worker
	//有空闲
	if n >= 0 {
		w = p.ready[n]
		p.ready[n] = nil
		p.ready = p.ready[:n]
		p.mu.Unlock()
		//说明用完了，重新创建worker
	} else if p.Running() < p.maxWorkers {
		p.mu.Unlock()
		w = p.pool.Get().(*worker)
		go p.runWorker(w)
	} else {
		//阻塞等待
	Reentry:
		p.blocking++
		p.cond.Wait()
		p.blocking--
		l := len(p.ready) - 1
		if l < 0 {
			goto Reentry
		}
		w = p.ready[l]
		p.ready[l] = nil
		p.ready = p.ready[:l]
		p.mu.Unlock()
	}

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
	p.running = 0
}
