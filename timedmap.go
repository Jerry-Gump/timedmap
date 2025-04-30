package timedmap

import (
	"sync"
	"time"
)

type callback func(value interface{})

// TimedMap contains a map with all key-value pairs,
// and a timer, which cleans the map in the set
// tick durations from expired keys.
type TimedMap[T any] struct {
	container        sync.Map
	elementPool      *sync.Pool
	cleanupTickcount int64
}

func (tm *TimedMap[T]) finalizer() {
	__CleanLoop.Remove(tm)
	tm.Flush()
}

func (tm *TimedMap[T]) Close() {
	tm.finalizer()
}

func (tm *TimedMap[T]) tickNow(tickcount int64) bool {
	return tickcount%tm.cleanupTickcount == 0
}

type keyWrap struct {
	sec int
	key interface{}
}

// element contains the actual value as interface type,
// the thime when the value expires and an array of
// callbacks, which will be executed when the element
// expires.
type element[T any] struct {
	value   T
	expires time.Time
	cbs     []callback
}

// New creates and returns a new instance of TimedMap.
// The passed cleanupTickTime will be passed to the
// cleanup ticker, which iterates through the map and
// deletes expired key-value pairs.
//
// Optionally, you can also pass a custom <-chan time.Time
// which controls the cleanup cycle if you want to use
// a single syncronyzed timer or if you want to have more
// control over the cleanup loop.
//
// When passing 0 as cleanupTickTime and no tickerChan,
// the cleanup loop will not be started. You can call
// StartCleanerInternal or StartCleanerExternal to
// manually start the cleanup loop. These both methods
// can also be used to re-define the specification of
// the cleanup loop when already running if you want to.
func New[T any](cleanupTime time.Duration) *TimedMap[T] {
	ctc := cleanupTime.Milliseconds() / 100
	if ctc < 1 {
		ctc = 1
	}
	tm := &TimedMap[T]{
		elementPool: &sync.Pool{
			New: func() interface{} {
				return new(element[T])
			},
		},
		cleanupTickcount: ctc,
	}
	__CleanLoop.Append(tm)

	return tm
}

// Section returns a sectioned subset of
// the timed map with the given section
// identifier i.
func (tm *TimedMap[T]) Section(i int) Section[T] {
	if i == 0 {
		return tm
	}
	return newSection(tm, i)
}

// Ident returns the current sections ident.
// In the case of the root object TimedMap,
// this is always 0.
func (tm *TimedMap[T]) Ident() int {
	return 0
}

// Set appends a key-value pair to the map or sets the value of
// a key. expiresAfter sets the expire time after the key-value pair
// will automatically be removed from the map.
func (tm *TimedMap[T]) Set(key interface{}, value T, expiresAfter time.Duration, cb ...callback) {
	tm.set(key, 0, value, expiresAfter, cb...)
}

// GetValue returns an interface of the value of a key in the
// map. The returned value is nil if there is no value to the
// passed key or if the value was expired.
func (tm *TimedMap[T]) GetValue(key interface{}) T {
	v := tm.get(key, 0)
	if v == nil {
		var r T
		return r
	}
	return v.value
}

// GetExpires returns the expire time of a key-value pair.
// If the key-value pair does not exist in the map or
// was expired, this will return an error object.
func (tm *TimedMap[T]) GetExpires(key interface{}) (time.Time, error) {
	v := tm.get(key, 0)
	if v == nil {
		return time.Time{}, ErrKeyNotFound
	}
	return v.expires, nil
}

// SetExpire is deprecated.
// Please use SetExpires instead.
func (tm *TimedMap[T]) SetExpire(key interface{}, d time.Duration) error {
	return tm.SetExpires(key, d)
}

// SetExpires sets the expire time for a key-value
// pair to the passed duration. If there is no value
// to the key passed , this will return an error.
func (tm *TimedMap[T]) SetExpires(key interface{}, d time.Duration) error {
	return tm.setExpires(key, 0, d)
}

// Contains returns true, if the key exists in the map.
// false will be returned, if there is no value to the
// key or if the key-value pair was expired.
func (tm *TimedMap[T]) Contains(key interface{}) bool {
	return tm.get(key, 0) != nil
}

// Remove deletes a key-value pair in the map.
func (tm *TimedMap[T]) Remove(key interface{}) {
	tm.remove(key, 0)
}

// Refresh extends the expire time for a key-value pair
// about the passed duration. If there is no value to
// the key passed, this will return an error object.
func (tm *TimedMap[T]) Refresh(key interface{}, d time.Duration) error {
	return tm.refresh(key, 0, d)
}

// Flush deletes all key-value pairs of the map.
func (tm *TimedMap[T]) Flush() {
	tm.container.Range(func(key, value interface{}) bool {
		tm.elementPool.Put(value)
		tm.container.Delete(key)
		return true
	})
}

// Size returns the current number of key-value pairs
// existent in the map.
func (tm *TimedMap[T]) Size() int {
	l := 0
	tm.container.Range(func(key, value interface{}) bool {
		l++
		return true
	})
	return l
}

// Snapshot returns a new map which represents the
// current key-value state of the internal container.
func (tm *TimedMap[T]) Snapshot() map[interface{}]T {
	return tm.getSnapshot(0)
}

// expireElement removes the specified key-value element
// from the map and executes all defined callback functions
func (tm *TimedMap[T]) expireElement(key interface{}, sec int, v *element[T]) {
	for _, cb := range v.cbs {
		if cb == nil {
			continue
		}
		cb(v.value)
	}

	k := keyWrap{
		sec: sec,
		key: key,
	}

	tm.elementPool.Put(v)
	tm.container.Delete(k)
}

// cleanUp iterates trhough the map and expires all key-value
// pairs which expire time after the current time
func (tm *TimedMap[T]) cleanUp() {
	now := time.Now()

	tm.container.Range(func(key, value any) bool {
		v := value.(*element[T])
		if now.After(v.expires) {
			k := key.(keyWrap)
			tm.expireElement(k.key, k.sec, v)
		}
		return true
	})
}

// set sets the value for a key and section with the
// given expiration parameters
func (tm *TimedMap[T]) set(key interface{}, sec int, val T, expiresAfter time.Duration, cb ...callback) {
	// re-use element when existent on this key
	if v := tm.getRaw(key, sec); v != nil {
		v.value = val
		v.expires = time.Now().Add(expiresAfter)
		if cb != nil {
			v.cbs = cb
		}
		return
	}

	k := keyWrap{
		sec: sec,
		key: key,
	}

	v := tm.elementPool.Get().(*element[T])
	v.value = val
	v.expires = time.Now().Add(expiresAfter)
	if cb != nil {
		v.cbs = cb
	}
	tm.container.Store(k, v)
}

// get returns an element object by key and section
// if the value has not already expired
func (tm *TimedMap[T]) get(key interface{}, sec int) *element[T] {
	v := tm.getRaw(key, sec)
	if v == nil {
		return nil
	}

	if time.Now().After(v.expires) {
		tm.expireElement(key, sec, v)
		return nil
	}

	return v
}

// getRaw returns the raw element object by key,
// not depending on expiration time
func (tm *TimedMap[T]) getRaw(key interface{}, sec int) *element[T] {
	k := keyWrap{
		sec: sec,
		key: key,
	}
	v, ok := tm.container.Load(k)
	if !ok {
		return nil
	}

	return v.(*element[T])
}

// remove removes an element from the map by giveb
// key and section
func (tm *TimedMap[T]) remove(key interface{}, sec int) {
	k := keyWrap{
		sec: sec,
		key: key,
	}

	v, ok := tm.container.Load(k)
	if !ok {
		return
	}

	tm.elementPool.Put(v)
	tm.container.Delete(k)
}

// refresh extends the lifetime of the given key in the
// given section by the duration d.
func (tm *TimedMap[T]) refresh(key interface{}, sec int, d time.Duration) error {
	v := tm.get(key, sec)
	if v == nil {
		return ErrKeyNotFound
	}
	v.expires = v.expires.Add(d)
	return nil
}

// setExpires sets the lifetime of the given key in the
// given section to the duration d.
func (tm *TimedMap[T]) setExpires(key interface{}, sec int, d time.Duration) error {
	v := tm.get(key, sec)
	if v == nil {
		return ErrKeyNotFound
	}
	v.expires = time.Now().Add(d)
	return nil
}

func (tm *TimedMap[T]) getSnapshot(sec int) (m map[interface{}]T) {
	m = make(map[interface{}]T)
	tm.container.Range(func(key, value any) bool {
		k := key.(keyWrap)
		if k.sec == sec {
			v := value.(*element[T])
			m[k.key] = v.value
		}
		return true
	})

	return
}
