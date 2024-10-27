# goroutine pool

tips: inspired by fast-http、ants

## 背景
虽然goroutine创建成本很低(内存仅占用2k)，可是如果不加控制，创建大量goroutine，内存占用会越来越高，极端情况下可能会导致OOM，而且大量的goroutine上下文切换，调度也会成为性能瓶颈。
## 开源实现
### fast-http
net/http库为每个连接分配2个goroutine，分别负责读写请求。随着连接数的增长, goroutine被大量创建，尤其在http短连接场景下，大量goroutine存在频繁的上下文切换，影响服务性能。fasthttp库通过协程池实现`复用`协程, 并且模块设计非常清晰易理解，除此之外作者也做了很多极致的内存优化，非常值得学习。

### ants
高性能的goroutine协程池,支持多种模式，是目前主流的协程池，同时也是非常好的学习资源。

综上纵使业界有不少成熟的组件，可是作为工程师不能存在拿来主义，要知其所以然，需要重点掌握`资源复用`的思想。



## 功能特性
- 高性能
- 接入门槛低
- 支持弹性扩缩goroutine



## 设计细节
### 获取worker
先从ready数组中获取，如果ready为空，此时需要判断当前worker数是否超过阈值，如果没有则从sync.Pool中new一个新的worker;
反之需要重新获取。

![img.png](img.png)

### 归还worker
当执行完task func之后，会将worker直接append到ready数组中
![img_1.png](img_1.png)

### 清理空闲worker
定期扫描ready数组，由于ready数组是FIFO模式, 因此可以采用二分查找快速定位空闲超时的worker.
![img_2.png](img_2.png)
## Example:

```shell
go get github.com/codingWhat/goroutinepool
```

```golang
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codingWhat/goroutinepool"
)

func main() {
	var wg sync.WaitGroup
	var sum atomic.Int32
	num := 1000
	wg.Add(num - 1)
	customFunc := func(a any) error {
		defer wg.Done()
		i := a.(int)
		sum.Add(int32(i))
		return nil
	}
	p := goroutinepool.NewWithFunc(100, customFunc)
	defer p.Release()
	for i := 1; i < num; i++ {
		p.Invoke(i)
	}
	time.Sleep(3 * time.Second)
	fmt.Printf("1 + %d = %d \n", num, sum.Load())
}	
```
