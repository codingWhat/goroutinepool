package goroutinepool

import (
	"sync"
	"testing"
	"time"

	"github.com/panjf2000/ants"
)

var num = 1000000

func BenchmarkAntsPool(b *testing.B) {
	// 创建一个协程池，包含1000个协程
	p, _ := ants.NewPool(num)
	wg := sync.WaitGroup{}
	defer p.Release()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		_ = p.Submit(func() {
			// 模拟任务处理时间
			time.Sleep(100 * time.Millisecond)
			wg.Done()
		})
	}
	b.StopTimer()
	// 等待所有任务被处理
	wg.Wait()
}

func BenchmarkWorkPool(b *testing.B) {
	wg := sync.WaitGroup{}
	wp := NewWithFunc(num, func(a any) error {
		time.Sleep(100 * time.Millisecond)
		wg.Done()
		return nil
	}, WithPreAlloc())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		wp.Invoke(i)
	}
	b.StopTimer()
	wg.Wait()
}
