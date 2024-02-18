/*
 * @Date: 2023-11-03 11:41:53
 * @LastEditors: Jerry Gump gongzengli@qq.com
 * @LastEditTime: 2023-11-03 15:21:06
 * @FilePath: e:\VSCode-Project\legalsoft.com.cn\timedmap\timedlist.go
 * @Description:
 * Copyright (c) 2023 by ${git_name} email: ${git_email}, All Rights Reserved.
 */
package timedmap

import (
	"sync"
	"time"
)

type TimedList[T any] struct {
	container   []*element[T]
	elementPool *sync.Pool

	cleanupTickTime time.Duration
	cleanerTicker   *time.Ticker
	cleanerStopChan chan bool
	cleanerRunning  bool
	mtx             *sync.RWMutex
}

func NewList[T any](cleanupTickTime time.Duration, tickerChan ...<-chan time.Time) *TimedList[T] {
	tm := &TimedList[T]{
		cleanerStopChan: make(chan bool),
		elementPool: &sync.Pool{
			New: func() interface{} {
				return new(element[T])
			},
		},
		mtx: &sync.RWMutex{},
	}

	if len(tickerChan) > 0 {
		tm.StartCleanerExternal(tickerChan[0])
	} else if cleanupTickTime > 0 {
		tm.cleanupTickTime = cleanupTickTime
		tm.StartCleanerInternal(cleanupTickTime)
	}

	return tm
} // Set appends a key-value pair to the map or sets the value of
// a key. expiresAfter sets the expire time after the key-value pair
// will automatically be removed from the map.
func (tm *TimedList[T]) Append(value T, expiresAfter time.Duration, cb ...callback) {
	tm.set(value, expiresAfter, cb...)
}

// GetValue returns an interface of the value of a key in the
// map. The returned value is nil if there is no value to the
// passed key or if the value was expired.
func (tm *TimedList[T]) GetValue(key int) T {
	v := tm.get(key)
	if v == nil {
		var r T
		return r
	}
	return v.value
}

// GetExpires returns the expire time of a key-value pair.
// If the key-value pair does not exist in the map or
// was expired, this will return an error object.
func (tm *TimedList[T]) GetExpires(key int) (time.Time, error) {
	v := tm.get(key)
	if v == nil {
		return time.Time{}, ErrKeyNotFound
	}
	return v.expires, nil
}

// SetExpire is deprecated.
// Please use SetExpires instead.
func (tm *TimedList[T]) SetExpire(key int, d time.Duration) error {
	return tm.setExpires(key, d)
}

// Remove deletes a key-value pair in the map.
func (tm *TimedList[T]) Remove(key int) {
	tm.remove(key)
}

// Refresh extends the expire time for a key-value pair
// about the passed duration. If there is no value to
// the key passed, this will return an error object.
func (tm *TimedList[T]) Refresh(key int, d time.Duration) error {
	return tm.refresh(key, d)
}

// Flush deletes all key-value pairs of the map.
func (tm *TimedList[T]) Flush() {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	for _, value := range tm.container {
		tm.elementPool.Put(value)
	}
	tm.container = nil
}

// Size returns the current number of key-value pairs
// existent in the map.
func (tm *TimedList[T]) Size() int {
	return len(tm.container)
}

// StartCleanerInternal starts the cleanup loop controlled
// by an internal ticker with the given interval.
//
// If the cleanup loop is already running, it will be
// stopped and restarted using the new specification.
func (tm *TimedList[T]) StartCleanerInternal(interval time.Duration) {
	if tm.cleanerRunning {
		tm.StopCleaner()
	}
	tm.cleanerTicker = time.NewTicker(interval)
	go tm.cleanupLoop(tm.cleanerTicker.C)
}

// StartCleanerExternal starts the cleanup loop controlled
// by the given initiator channel. This is useful if you
// want to have more control over the cleanup loop or if
// you want to sync up multiple timedmaps.
//
// If the cleanup loop is already running, it will be
// stopped and restarted using the new specification.
func (tm *TimedList[T]) StartCleanerExternal(initiator <-chan time.Time) {
	if tm.cleanerRunning {
		tm.StopCleaner()
	}
	go tm.cleanupLoop(initiator)
}

// StopCleaner stops the cleaner go routine and timer.
// This should always be called after exiting a scope
// where TimedMap is used that the data can be cleaned
// up correctly.
func (tm *TimedList[T]) StopCleaner() {
	if !tm.cleanerRunning {
		return
	}
	tm.cleanerStopChan <- true
	if tm.cleanerTicker != nil {
		tm.cleanerTicker.Stop()
	}
}

// Snapshot returns a new map which represents the
// current key-value state of the internal container.
func (tm *TimedList[T]) Snapshot() []T {
	return tm.getSnapshot()
}

// cleanupLoop holds the loop executing the cleanup
// when initiated by tc.
func (tm *TimedList[T]) cleanupLoop(tc <-chan time.Time) {
	tm.cleanerRunning = true
	defer func() {
		tm.cleanerRunning = false
	}()

	for {
		select {
		case <-tc:
			tm.cleanUp()
		case <-tm.cleanerStopChan:
			return
		}
	}
}

// expireElement removes the specified key-value element
// from the map and executes all defined callback functions
func (tm *TimedList[T]) expireElement(idx int, v *element[T]) {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	for _, cb := range v.cbs {
		if cb == nil {
			continue
		}
		cb(v.value)
	}

	tm.elementPool.Put(v)
	tm.container = append(tm.container[:idx], tm.container[:idx+1]...)
}

// cleanUp iterates trhough the map and expires all key-value
// pairs which expire time after the current time
func (tm *TimedList[T]) cleanUp() {
	now := time.Now()

	for key, value := range tm.container {
		if now.After(value.expires) {
			tm.expireElement(key, value)
		}
	}
}

// set sets the value for a key and section with the
// given expiration parameters
func (tm *TimedList[T]) set(val T, expiresAfter time.Duration, cb ...callback) {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	v := tm.elementPool.Get().(*element[T])
	v.value = val
	v.expires = time.Now().Add(expiresAfter)
	if cb != nil {
		v.cbs = cb
	}
	tm.container = append(tm.container, v)
}

// get returns an element object by key and section
// if the value has not already expired
func (tm *TimedList[T]) get(key int) *element[T] {
	v := tm.getRaw(key)
	if v == nil {
		return nil
	}

	if time.Now().After(v.expires) {
		tm.expireElement(key, v)
		return nil
	}

	return v
}

// getRaw returns the raw element object by key,
// not depending on expiration time
func (tm *TimedList[T]) getRaw(key int) *element[T] {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()
	if key >= 0 && key < len(tm.container) {
		v := tm.container[key]
		return v
	} else {
		return nil
	}
}

// remove removes an element from the map by giveb
// key and section
func (tm *TimedList[T]) remove(key int) {
	v := tm.getRaw(key)
	if v == nil {
		return
	}

	tm.elementPool.Put(v)
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	tm.container = append(tm.container[:key], tm.container[:key+1]...)
}

// refresh extends the lifetime of the given key in the
// given section by the duration d.
func (tm *TimedList[T]) refresh(key int, d time.Duration) error {
	v := tm.get(key)
	if v == nil {
		return ErrKeyNotFound
	}
	v.expires = v.expires.Add(d)
	return nil
}

// setExpires sets the lifetime of the given key in the
// given section to the duration d.
func (tm *TimedList[T]) setExpires(key int, d time.Duration) error {
	v := tm.get(key)
	if v == nil {
		return ErrKeyNotFound
	}
	v.expires = time.Now().Add(d)
	return nil
}

func (tm *TimedList[T]) getSnapshot() (m []T) {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()
	m = make([]T, len(tm.container))
	for _, value := range tm.container {
		m = append(m, value.value)
	}

	return
}
