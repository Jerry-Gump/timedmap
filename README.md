<div align="center">
    <h1>~ timedmap ~</h1>
    <strong>A map which has expiring key-value pairs.</strong><br><br>
    <a href="https://pkg.go.dev/github.com/Jerry-Gump/timedmap"><img src="https://godoc.org/github.com/Jerry-Gump/timedmap?status.svg" /></a>&nbsp;
    <a href="https://github.com/Jerry-Gump/timedmap/actions/workflows/main-ci.yml" ><img src="https://github.com/Jerry-Gump/timedmap/actions/workflows/main-ci.yml/badge.svg" /></a>&nbsp;
    <a href="https://coveralls.io/github/Jerry-Gump/timedmap"><img src="https://coveralls.io/repos/github/Jerry-Gump/timedmap/badge.svg" /></a>&nbsp;
    <a href="https://goreportcard.com/report/github.com/Jerry-Gump/timedmap"><img src="https://goreportcard.com/badge/github.com/Jerry-Gump/timedmap"/></a>&nbsp;
	<a href="https://github.com/avelino/awesome-go"><img src="https://awesome.re/mentioned-badge.svg"/></a>
<br>
</div>

---

<div align="center">
    <code>go get -u github.com/Jerry-Gump/timedmap</code>
</div>

---

## Intro
来自于https://github.com/zekroTJA/timedmap。

废弃了原先使用map[keyWrap]*element，并且使用RWMutex进行数据同步的方案，采用了sync.Map来进行存储。
不知为何，在我的环境里，如果超时时长较长的情况下，RWMutex会死锁。

另外在超时回收函数里，对回调函数指针进行了检查，避免错误使用录入了空指针的异常。

最新增加了对泛型的支持，使用更方便，如果需要在Value里面使用多种类型，则需要使用 New[any](...)实现更多类型支持

This package allows to set values to a map which will expire and disappear after a specified time.

[Here](https://pkg.go.dev/github.com/zekroTJA/timedmap) you can read the docs of this package, generated by pkg.go.dev.

---

## Usage Example

```go
package main

import (
	"log"
	"time"

	"github.com/Jerry-Gump/timedmap"
)

func main() {
	// Create a timed map with a cleanup timer interval of 1 second
	tm := timedmap.New(1 * time.Second)
	// Set value of key "hey" to 213, which will expire after 3 seconds
	tm.Set("hey", 213, 3*time.Second)
	// Print the value of "hey"
	printKeyVal(tm, "hey")
	// Block the main thread for 5 seconds
	// After this time, the key-value pair "hey": 213 has expired
	time.Sleep(5 * time.Second)
	// Now, this function should show that there is no key "hey"
	// in the map, because it has been expired
	printKeyVal(tm, "hey")
}

func printKeyVal(tm *timedmap.TimedMap, key interface{}) {
	d, ok := tm.GetValue(key).(int)
	if !ok {
		log.Println("data expired")
		return
	}

	log.Printf("%v = %d\n", key, d)
}
```

Further examples, you can find in the [example](examples) directory.

If you want to see this package in a practcal use case scenario, please take a look at the rate limiter implementation of the REST API of [myrunes.com](https://myrunes.com), where I have used `timedmap` for storing client-based limiter instances:  
https://github.com/myrunes/backend/blob/master/internal/ratelimit/ratelimit.go

---

Copyright (c) 2020 zekro Development (Ringo Hoffmann).  
Covered by MIT licence.
