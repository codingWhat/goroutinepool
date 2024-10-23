package goroutinepool

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	_   = 1 << (10 * iota)
	KiB // 1024
	MiB // 1048576
)

func TestNewWithFunc(t *testing.T) {
	var sum atomic.Int32
	wg := sync.WaitGroup{}
	num := 1000
	wg.Add(num - 1)
	p := NewWithFunc(100, func(a any) error {
		defer wg.Done()

		i := a.(int)
		sum.Add(int32(i))
		return nil
	})
	defer p.Release()
	for i := 1; i < num; i++ {
		p.Invoke(i)
	}
	wg.Wait()
	fmt.Println("sum----->", sum.Load())
}

func TestNewWithFunc_Purge(t *testing.T) {
	var sum atomic.Int32
	wg := sync.WaitGroup{}
	num := 10
	wg.Add(num - 1)
	custom := func(a any) error {
		defer wg.Done()

		i := a.(int)
		sum.Add(int32(i))
		return nil
	}

	p := NewWithFunc(10, custom, WithMaxIdleTime(3*time.Second))
	defer p.Release()
	for i := 1; i < num; i++ {
		p.Invoke(i)
	}
	wg.Wait()

	time.Sleep(6 * time.Second)
	assert.Equal(t, p.Running(), 0)
}

var curMem uint64

func TestAntsPoolWithFuncWaitToGetWorkerPreMalloc(t *testing.T) {
	var wg sync.WaitGroup
	var sum atomic.Int32

	num := 100000
	p := NewWithFunc(1000, func(a any) error {
		defer wg.Done()

		i := a.(int)
		sum.Add(int32(i))
		time.Sleep(time.Duration(1) * time.Millisecond)
		return nil
	})
	defer p.Release()

	for i := 0; i < num; i++ {
		wg.Add(1)
		p.Invoke(i)
	}
	wg.Wait()
	t.Logf("pool with func, running workers number:%d", p.Running())
	mem := runtime.MemStats{}
	runtime.ReadMemStats(&mem)
	curMem = mem.TotalAlloc/MiB - curMem
	t.Logf("memory usage:%d MB", curMem)
}
