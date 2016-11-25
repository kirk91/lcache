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

// New create a cache container with default capacity and given parameters.
func New(fn interface{}, ttl time.Duration) (*Container, error) {
	return newContainer(DefaultCapacity, fn, ttl)
}

// NewWithSize constructs a cache container with the given parameters.
func NewWithSize(size int, fn interface{}, ttl time.Duration) (*Container, error) {
	if size < 0 {
		return nil, errors.New("Must provide a positive size")
	}
	return newContainer(size, fn, ttl)
}

// Must is a helper that wraps a call to a function returning (*Container, error)
// and panics if the error is non-nil. It is intended for use in variable initializations
// such as
//		var c = lcache.Must(lcache.New(func(){}, time.Minute))
func Must(c *Container, err error) *Container {
	if err != nil {
		panic(err)
	}
	return c
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

// Item is used to hold a value
type Item struct {
	key        string
	params     []interface{}
	value      interface{}
	ttl        time.Duration
	expire     time.Time
	fn         interface{}
	initialed  bool
	initialCh  chan struct{}
	refreshing bool
	mu         sync.Mutex
}

// NewItem constructs an item of the given parameters
func NewItem(params []interface{}, ttl time.Duration, fn interface{}) *Item {
	return &Item{key: fmt.Sprintf("%v", params),
		params:    params,
		ttl:       ttl,
		fn:        fn,
		initialCh: make(chan struct{}),
	}
}

// Value returns the real value in the item. If real value has been loaded,
// it will return immediately. Otherwise, it will return until the real value
// is initialed.
func (i *Item) Value() (val interface{}) {
	if time.Now().Before(i.expire) {
		return i.value
	}
	i.Refresh()
	// if item has not initialed, wait until initial done.
	// else return old value directly
	if !i.initialed {
		<-i.initialCh
	}
	return i.value
}

// Refresh is used to refresh real value with fn callback.
func (i *Item) Refresh() {
	i.mu.Lock()
	if i.refreshing {
		i.mu.Unlock()
		return
	}
	i.refreshing = true
	go i.refresh()
	i.mu.Unlock()
	return
}

func (i *Item) refresh() {
	// load data with fn
	if val, err := i.loadData(); err == nil {
		i.value = val
	}

	i.expire = time.Now().Add(i.ttl)
	// reset refresh flag
	i.refreshing = false
	// set initialed flag
	if !i.initialed {
		i.initialed = true
		close(i.initialCh)
	}
}

// loadData is used to load data with fn and params
func (i *Item) loadData() (interface{}, error) {
	f := reflect.ValueOf(i.fn)
	in := make([]reflect.Value, f.Type().NumIn())
	for k, param := range i.params {
		in[k] = reflect.ValueOf(param)
	}
	res := f.Call(in)
	if res[1].Interface() == nil {
		return res[0].Interface(), nil
	}
	return res[0].Interface(), res[1].Interface().(error)
}
