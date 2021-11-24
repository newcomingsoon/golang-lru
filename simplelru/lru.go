package simplelru

import (
	"container/list"
	"errors"
)

// EvictCallback is used to get a callback when a cache entry is evicted
// 当有key淘汰时，会触发的回调函数（如果初始化LRU时有注册的回调函数的话）
type EvictCallback func(key interface{}, value interface{})

// LRU implements a non-thread safe fixed size LRU cache
// 注意： 此处实现的simpleLRU是线程不安全的
// 根目录lru.go 通过新增一个RWMutex锁，包装simpleLRU就实现了一个线程安全的
type LRU struct {
	// LRU大小
	size      int
	// 通过list双向链表来维护元素
	// 好处在于：里面封装好了队首，队尾，将指定元素移动到队头，删除队尾元素等操作
	// 这样很多元素的操作都使用了list本身就存在的一些函数，极大方便了操作
	evictList *list.List
	// 保存具体 key-value的map， value的值就是存储在list中的具体元素
	items     map[interface{}]*list.Element
	// 注册的回调函数，如果有的话
	onEvict   EvictCallback
}

// entry is used to hold a value in the evictList
// list.Element 中Value具体存储的元素内容就是该entry定义的
type entry struct {
	key   interface{}
	value interface{}
}

// NewLRU constructs an LRU of the given size
// 构造LRU，初始化对象
func NewLRU(size int, onEvict EvictCallback) (*LRU, error) {
	if size <= 0 {
		return nil, errors.New("Must provide a positive size")
	}
	c := &LRU{
		size:      size,
		evictList: list.New(),
		items:     make(map[interface{}]*list.Element),
		onEvict:   onEvict,
	}
	return c, nil
}

// Purge is used to completely clear the cache.
// 清空LRU
func (c *LRU) Purge() {
	for k, v := range c.items {
		// 每个元素淘汰的时候，如果有注册回调函数的话，都应该在此刻调用
		if c.onEvict != nil {
			c.onEvict(k, v.Value.(*entry).value)
		}
		// 删除map中的key
		delete(c.items, k)
	}
	// 清空list
	c.evictList.Init()
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (c *LRU) Add(key, value interface{}) (evicted bool) {
	// Check for existing item
	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		ent.Value.(*entry).value = value
		return false
	}

	// Add new item
	ent := &entry{key, value}
	// items map中的value和list中的元素都是同一个地址引用
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
		c.evictList.MoveToFront(ent)
		if ent.Value.(*entry) == nil {
			return nil, false
		}
		return ent.Value.(*entry).value, true
	}
	return
}

// Contains checks if a key is in the cache, without updating the recent-ness
// or deleting it for being stale.
// 只判断是否存在，不算真正的一次访问该元素
// 区别在于： 不算访问的话，不会将该元素移动到队头，越是在队头代表元素也是最近被访问
// 越是最晚才会被淘汰
func (c *LRU) Contains(key interface{}) (ok bool) {
	_, ok = c.items[key]
	return ok
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *LRU) Peek(key interface{}) (value interface{}, ok bool) {
	var ent *list.Element
	if ent, ok = c.items[key]; ok {
		return ent.Value.(*entry).value, true
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
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
		kv := ent.Value.(*entry)
		return kv.key, kv.value, true
	}
	return nil, nil, false
}

// GetOldest returns the oldest entry
func (c *LRU) GetOldest() (key interface{}, value interface{}, ok bool) {
	ent := c.evictList.Back()
	if ent != nil {
		kv := ent.Value.(*entry)
		return kv.key, kv.value, true
	}
	return nil, nil, false
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *LRU) Keys() []interface{} {
	keys := make([]interface{}, len(c.items))
	i := 0
	for ent := c.evictList.Back(); ent != nil; ent = ent.Prev() {
		keys[i] = ent.Value.(*entry).key
		i++
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
