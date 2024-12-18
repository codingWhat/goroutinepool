package goroutinepool

import "time"

//type Options struct {
//	WorkerMaxIdleTime time.Duration
//}

type Option func(p *poolManager)

func WithMaxIdleTime(t time.Duration) Option {
	return func(p *poolManager) {
		p.workerMaxIdleTime = t
	}
}

func WithPreAlloc() Option {
	return func(p *poolManager) {
		p.ready = make([]*worker, 0, p.maxWorkers)
	}
}
