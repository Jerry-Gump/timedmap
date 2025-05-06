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

	cleanupTickcount int64
	mtx              *sync.RWMutex
}

// 因为牵涉到引用计数的问题，所以实际上runtime.SetFinalizer在这里不起作用，所以需要主动Close
func (tl *TimedList[T]) finalizer() {
	__CleanLoop.Remove(tl)
	tl.Flush()
}

func (tl *TimedList[T]) Close() {
	tl.finalizer()
}

func (tl *TimedList[T]) tickNow(tickcount int64) bool {
	return tickcount%tl.cleanupTickcount == 0
}

func NewList[T any](cleanupTime time.Duration) *TimedList[T] {
	ctc := cleanupTime.Milliseconds() / 100
	if ctc < 1 {
		ctc = 1
	}
	tm := &TimedList[T]{
		elementPool: &sync.Pool{
			New: func() interface{} {
				return new(element[T])
			},
		},
		cleanupTickcount: ctc,
		mtx:              &sync.RWMutex{},
	}
	__CleanLoop.Append(tm)

	return tm
}

// Set appends a key-value pair to the map or sets the value of
// a key. expiresAfter sets the expire time after the key-value pair
// will automatically be removed from the map.
func (tm *TimedList[T]) Append(value T, expiresAfter time.Duration, cb ...callback) {
	tm.set(value, expiresAfter, cb...)
}

// GetValue returns an interface of the value of a key in the
// map. The returned value is nil if there is no value to the
// passed key or if the value was expired.
func (tm *TimedList[T]) GetValue(index int) T {
	v := tm.get(index)
	if v == nil {
		var r T
		return r
	}
	return v.value
}

// GetExpires returns the expire time of a key-value pair.
// If the key-value pair does not exist in the map or
// was expired, this will return an error object.
func (tm *TimedList[T]) GetExpires(index int) (time.Time, error) {
	v := tm.get(index)
	if v == nil {
		return time.Time{}, ErrKeyNotFound
	}
	return v.expires, nil
}

// SetExpire is deprecated.
// Please use SetExpires instead.
func (tm *TimedList[T]) SetExpire(index int, d time.Duration) error {
	return tm.setExpires(index, d)
}

// Remove deletes a value with index of slice.
func (tm *TimedList[T]) Remove(index int) {
	tm.remove(index)
}

// Refresh extends the expire time for a key-value pair
// about the passed duration. If there is no value to
// the key passed, this will return an error object.
func (tm *TimedList[T]) Refresh(index int, d time.Duration) error {
	return tm.refresh(index, d)
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
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	return len(tm.container)
}

// Snapshot returns a new map which represents the
// current key-value state of the internal container.
func (tm *TimedList[T]) Snapshot() []T {
	return tm.getSnapshot()
}

// expireElement removes the specified key-value element
// from the map and executes all defined callback functions
func (tm *TimedList[T]) expireElement(index int, v *element[T]) {
	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	for _, cb := range v.cbs {
		if cb == nil {
			continue
		}
		cb(v.value)
	}

	tm.elementPool.Put(v)
	tm.container = append(tm.container[:index], tm.container[index+1:]...)
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
func (tm *TimedList[T]) get(index int) *element[T] {
	v := tm.getRaw(index)
	if v == nil {
		return nil
	}

	if time.Now().After(v.expires) {
		tm.expireElement(index, v)
		return nil
	}

	return v
}

// getRaw returns the raw element object by key,
// not depending on expiration time
func (tm *TimedList[T]) getRaw(index int) *element[T] {
	tm.mtx.RLock()
	defer tm.mtx.RUnlock()
	if index >= 0 && index < len(tm.container) {
		v := tm.container[index]
		return v
	} else {
		return nil
	}
}

// remove removes an element from the map by giveb
// key and section
func (tm *TimedList[T]) remove(index int) {
	v := tm.getRaw(index)
	if v == nil {
		return
	}

	tm.mtx.Lock()
	defer tm.mtx.Unlock()
	tm.elementPool.Put(v)
	tm.container = append(tm.container[:index], tm.container[index+1:]...)
}

// refresh extends the lifetime of the given key in the
// given section by the duration d.
func (tm *TimedList[T]) refresh(index int, d time.Duration) error {
	v := tm.get(index)
	if v == nil {
		return ErrKeyNotFound
	}
	v.expires = v.expires.Add(d)
	return nil
}

// setExpires sets the lifetime of the given key in the
// given section to the duration d.
func (tm *TimedList[T]) setExpires(index int, d time.Duration) error {
	v := tm.get(index)
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
