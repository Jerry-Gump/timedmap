package timedmap

import (
	"time"
)

// Section defines a sectioned access
// wrapper of TimedMap.
type Section[T any] interface {

	// Ident returns the current sections identifier
	Ident() int

	// Set appends a key-value pair to the map or sets the value of
	// a key. expiresAfter sets the expire time after the key-value pair
	// will automatically be removed from the map.
	Set(key interface{}, value T, expiresAfter time.Duration, cb ...callback)

	// GetValue returns an interface of the value of a key in the
	// map. The returned value is nil if there is no value to the
	// passed key or if the value was expired.
	GetValue(key interface{}) T

	// GetExpires returns the expire time of a key-value pair.
	// If the key-value pair does not exist in the map or
	// was expired, this will return an error object.
	GetExpires(key interface{}) (time.Time, error)

	// SetExpires sets the expire time for a key-value
	// pair to the passed duration. If there is no value
	// to the key passed , this will return an error.
	SetExpires(key interface{}, d time.Duration) error

	// Contains returns true, if the key exists in the map.
	// false will be returned, if there is no value to the
	// key or if the key-value pair was expired.
	Contains(key interface{}) bool

	// Remove deletes a key-value pair in the map.
	Remove(key interface{})

	// Refresh extends the expire time for a key-value pair
	// about the passed duration. If there is no value to
	// the key passed, this will return an error.
	Refresh(key interface{}, d time.Duration) error

	// Flush deletes all key-value pairs of the section
	// in the map.
	Flush()

	// Size returns the current number of key-value pairs
	// existent in the section of the map.
	Size() (i int)

	// Snapshot returns a new map which represents the
	// current key-value state of the internal container.
	Snapshot() map[interface{}]T
}

// section wraps access to a specific
// section of the map.
type section[T any] struct {
	tm  *TimedMap[T]
	sec int
}

// newSection creates a new Section instance
// wrapping the given TimedMap instance and
// section identifier.
func newSection[T any](tm *TimedMap[T], sec int) *section[T] {
	return &section[T]{
		tm:  tm,
		sec: sec,
	}
}

func (s *section[T]) Ident() int {
	return s.sec
}

func (s *section[T]) Set(key interface{}, value T, expiresAfter time.Duration, cb ...callback) {
	s.tm.set(key, s.sec, value, expiresAfter, cb...)
}

func (s *section[T]) GetValue(key interface{}) T {
	v := s.tm.get(key, s.sec)
	if v == nil {
		var r T
		return r
	}
	return v.value
}

func (s *section[T]) GetExpires(key interface{}) (time.Time, error) {
	v := s.tm.get(key, s.sec)
	if v == nil {
		return time.Time{}, ErrKeyNotFound
	}
	return v.expires, nil
}

func (s *section[T]) SetExpires(key interface{}, d time.Duration) error {
	return s.tm.setExpires(key, s.sec, d)
}

func (s *section[T]) Contains(key interface{}) bool {
	return s.tm.get(key, s.sec) != nil
}

func (s *section[T]) Remove(key interface{}) {
	s.tm.remove(key, s.sec)
}

func (s *section[T]) Refresh(key interface{}, d time.Duration) error {
	return s.tm.refresh(key, s.sec, d)
}

func (s *section[T]) Flush() {
	s.tm.container.Range(func(key, value any) bool {
		k := key.(keyWrap)
		if k.sec == s.sec {
			s.tm.remove(k.key, k.sec)
		}
		return true
	})
}

func (s *section[T]) Size() (i int) {
	s.tm.container.Range(func(key, value any) bool {
		k := key.(keyWrap)
		if k.sec == s.sec {
			i++
		}
		return true
	})
	return
}

func (s *section[T]) Snapshot() map[interface{}]T {
	return s.tm.getSnapshot(s.sec)
}
