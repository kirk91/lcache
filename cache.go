package lcache

import (
	"container/list"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

const (
	// DefaultCapacity is the default size of container
	DefaultCapacity = 512
)

var (
	// ErrInvalidFn indicates the given fn is invalid
	ErrInvalidFn = errors.New("invalid cache fn")
	// ErrFnParams indicates the given parameters not matched with fn callback
	ErrFnParams = errors.New("cache fn params not adapted")
)

// Container implements a thread-safe cache container
type Container struct {
	sync.RWMutex
	capacity  int
	fn        interface{}
	fnKind    reflect.Kind
	fnNumIn   int
	fnNumOut  int
	ttl       time.Duration
	items     map[string]*list.Element
	evictList *list.List
}

// NewContainer create a cache container with default capacity and given parameters.
func NewContainer(fn interface{}, ttl time.Duration) (*Container, error) {
	return newContainer(DefaultCapacity, fn, ttl)
}

// NewContainerWithSize constructs a cache container with the given parameters.
func NewContainerWithSize(size int, fn interface{}, ttl time.Duration) (*Container, error) {
	if size < 0 {
		return nil, errors.New("Must provide a positive size")
	}
	return newContainer(size, fn, ttl)
}

func newContainer(size int, fn interface{}, ttl time.Duration) (*Container, error) {
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func || t.NumOut() != 2 {
		return nil, ErrInvalidFn
	}
	c := &Container{
		capacity:  size,
		fn:        fn,
		fnKind:    t.Kind(),
		fnNumIn:   t.NumIn(),
		fnNumOut:  t.NumOut(),
		ttl:       ttl,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
	return c, nil
}

// Get is used to obtain the value with the given parameters. If the params string
// has in the container, it will return immediately. Otherwise, it will load data
// with the fn callback.
func (c *Container) Get(params ...interface{}) (interface{}, error) {
	// check params
	if len(params) != c.fnNumIn {
		return nil, ErrFnParams
	}

	key := fmt.Sprintf("%v", params)
	c.Lock()
	defer c.Unlock()
	ent, ok := c.items[key]
	if ok {
		c.evictList.MoveToFront(ent)
		return ent.Value.(*Item).Value(), nil
	}

	// add new item
	item := NewItem(params, c.ttl, c.fn)
	ent = c.evictList.PushFront(item)
	c.items[key] = ent

	evict := c.evictList.Len() > c.capacity
	if evict {
		c.removeOldest()
	}
	return item.Value(), nil
}

// removeOldest removes the oldest item from the container.
func (c *Container) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the container.
func (c *Container) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	item := e.Value.(*Item)
	delete(c.items, item.key)
}

// Purge is used to completely clear the container
func (c *Container) Purge() {
	c.Lock()
	defer c.Unlock()
	for key := range c.items {
		delete(c.items, key)
	}
	c.evictList.Init()
}

// Remove removes the provided params from the container, returning if the
// params key was contained.
func (c *Container) Remove(params ...interface{}) bool {
	key := fmt.Sprintf("%v", params)
	c.Lock()
	defer c.Unlock()
	if ent, ok := c.items[key]; ok {
		c.removeElement(ent)
		return true
	}
	return false
}

// Len returns the number of items in the container
func (c *Container) Len() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.items)
}
