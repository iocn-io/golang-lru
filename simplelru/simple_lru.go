package simplelru

import (
	"container/list"
	"errors"
	"time"
)

// EvictCallback is used to get a callback when a cache entry is evicted
type EvictCallback func(key interface{}, value interface{})

// LRU implements a non-thread safe fixed size LRU cache
type LRU struct {
	size      int
	evictList *list.List
	items     map[interface{}]*list.Element
	expire    time.Duration
	onEvict   EvictCallback
}

// entry is used to hold a value in the evictList
type entry struct {
	key    interface{}
	value  interface{}
	expire *time.Time
}

// NewLRU constructs an LRU of the given size
func NewLRU(size int, onEvict EvictCallback) (*LRU, error) {
	return NewLRUWithExpire(size, 0, onEvict)
}

func NewLRUWithExpire(size int, expire time.Duration, onEvict EvictCallback) (*LRU, error) {
	if size <= 0 {
		return nil, errors.New("Must provide a positive size")
	}
	c := &LRU{
		size:      size,
		evictList: list.New(),
		items:     make(map[interface{}]*list.Element),
		expire:    expire,
		onEvict:   onEvict,
	}
	return c, nil
}

func (e *entry) IsExpired() bool {
	if e.expire == nil {
		return false
	}
	return time.Now().After(*e.expire)
}

// Purge is used to completely clear the cache.
func (c *LRU) Purge() {
	for k, v := range c.items {
		if c.onEvict != nil {
			c.onEvict(k, v.Value.(*entry).value)
		}
		delete(c.items, k)
	}
	c.evictList.Init()
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (c *LRU) Add(key, value interface{}) (evicted bool) {
	return c.AddEx(key, value, 0)
}

func (c *LRU) AddEx(key, value interface{}, expire time.Duration) (evicted bool) {
	var ex *time.Time = nil
	if expire > 0 {
		expire := time.Now().Add(expire)
		ex = &expire
	} else if c.expire > 0 {
		expire := time.Now().Add(c.expire)
		ex = &expire
	}

	// Check for existing item
	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		v := ent.Value.(*entry)
		v.value = value
		v.expire = ex
		return false
	}

	// Add new item
	ent := &entry{key, value, ex}
	entry := c.evictList.PushFront(ent)
	c.items[key] = entry

	evict := c.evictList.Len() > c.size
	// Verify size not exceeded
	if evict {
		c.removeOldest()
	}
	return evict
}

// Get looks up a key's value from the cache.
func (c *LRU) Get(key interface{}) (value interface{}, ok bool) {
	if ent, ok := c.items[key]; ok {
		v := ent.Value.(*entry)
		if v.IsExpired() {
			return nil, false
		}
		c.evictList.MoveToFront(ent)
		if v == nil {
			return nil, false
		}
		return v.value, true
	}
	return
}

// Contains checks if a key is in the cache, without updating the recent-ness
// or deleting it for being stale.
func (c *LRU) Contains(key interface{}) bool {
	if ent, ok := c.items[key]; ok {
		return !ent.Value.(*entry).IsExpired()
	}
	return false
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *LRU) Peek(key interface{}) (value interface{}, ok bool) {
	var ent *list.Element
	if ent, ok = c.items[key]; ok {
		v := ent.Value.(*entry)
		if v.IsExpired() {
			return nil, false
		}
		return v.value, true
	}
	return nil, ok
}

// Remove removes the provided key from the cache, returning if the
// key was contained.
func (c *LRU) Remove(key interface{}) (present bool) {
	if ent, ok := c.items[key]; ok {
		c.removeElement(ent)
		return true
	}
	return false
}

// RemoveOldest removes the oldest item from the cache.
func (c *LRU) RemoveOldest() (key interface{}, value interface{}, ok bool) {
LOOP:
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
		kv := ent.Value.(*entry)
		if kv.IsExpired() {
			goto LOOP
		}
		return kv.key, kv.value, true
	}
	return nil, nil, false
}

// GetOldest returns the oldest entry
func (c *LRU) GetOldest() (key interface{}, value interface{}, ok bool) {
LOOP:
	ent := c.evictList.Back()
	if ent != nil {
		kv := ent.Value.(*entry)
		if kv.IsExpired() {
			c.removeElement(ent.Next())
			goto LOOP
		}
		return kv.key, kv.value, true
	}
	return nil, nil, false
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *LRU) Keys() []interface{} {
	keys := make([]interface{}, 0, len(c.items))
	var v *entry
	for ent := c.evictList.Back(); ent != nil; ent = ent.Prev() {
		v = ent.Value.(*entry)
		if v.IsExpired() {
			continue
		}
		keys = append(keys, v.key)
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *LRU) Len() int {
	return c.evictList.Len()
}

// Resize changes the cache size.
func (c *LRU) Resize(size int) (evicted int) {
	diff := c.Len() - size
	if diff < 0 {
		diff = 0
	}
	for i := 0; i < diff; i++ {
		c.removeOldest()
	}
	c.size = size
	return diff
}

// removeOldest removes the oldest item from the cache.
func (c *LRU) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *LRU) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	kv := e.Value.(*entry)
	delete(c.items, kv.key)
	if c.onEvict != nil {
		c.onEvict(kv.key, kv.value)
	}
}
