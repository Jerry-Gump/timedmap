package timedmap

import (
	"runtime"
	"sync"
	"time"
)

type timedContainer interface {
	finalizer()
	cleanUp()
	tickNow(tickcount int64) bool
}

type cleanLoop struct {
	listCleanerTicker *time.Ticker
	timedContainers   []timedContainer
	mtx               *sync.Mutex
	onece             sync.Once
	tickcount         int64
}

func (lcl *cleanLoop) Append(c timedContainer) {
	lcl.mtx.Lock()
	defer lcl.mtx.Unlock()
	for _, tc := range lcl.timedContainers {
		if tc == c {
			return
		}
	}
	runtime.SetFinalizer(c, func(t timedContainer) {
		t.finalizer()
	})
	lcl.timedContainers = append(lcl.timedContainers, c)
}

func (lcl *cleanLoop) Remove(c timedContainer) {
	lcl.mtx.Lock()
	defer lcl.mtx.Unlock()
	for i, tc := range lcl.timedContainers {
		if tc == c {
			lcl.timedContainers = append(lcl.timedContainers[:i], lcl.timedContainers[i+1:]...)
			return
		}
	}
}

func (lcl *cleanLoop) cleanLoop() {
	for range lcl.listCleanerTicker.C {
		lcl.tickcount++
		
		for _, tc := range lcl.timedContainers {
			if tc.tickNow(lcl.tickcount) {
				go tc.cleanUp()
			}
		}
	}
}

func (lcl *cleanLoop) start() {
	lcl.onece.Do(func() {
		lcl.mtx = &sync.Mutex{}
		lcl.listCleanerTicker = time.NewTicker(time.Millisecond * 100)
		go lcl.cleanLoop()
	})
}

var __CleanLoop cleanLoop

func init() {
	__CleanLoop = cleanLoop{}
	__CleanLoop.start()
}
