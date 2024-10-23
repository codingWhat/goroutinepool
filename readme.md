# goroutine pool

tips: inspired by fast-http、ants

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
```
customFunc := func(a any) error {
    defer wg.Done()

        i := a.(int)
        sum.Add(int32(i))
        return nil
}
    
var sum atomic.Int32
num := 1000
p := NewWithFunc(100, customFunc)
defer p.Release()
for i := 1; i < num; i++ {
    p.Invoke(i)
}

time.Sleep(10 * time.Second)
```
