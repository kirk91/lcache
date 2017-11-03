package lcache

import (
	"bytes"
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

	// ErrResourceExhausted indicates underlying resouce exhausted, the response from underlying
	// api or interface won't be cached.
	ErrResourceExhausted = errors.New("resouce exhausted")
)

type options struct {
	cacheKeyGenerator CacheKeyGenerator
	enableLRU         bool
	capacity          int
}

type Option func(*options)

func WithCacheKeyGenerator(g CacheKeyGenerator) Option {
	return func(o *options) {
		o.cacheKeyGenerator = g
	}
}

func WithLRU() Option {
	return func(o *options) {
		o.enableLRU = true
	}
}

func WithCapacity(capacity int) Option {
	return func(o *options) {
		o.capacity = capacity
	}
}

// Container implements a thread-safe cache container
type Container struct {
	sync.RWMutex
	opts *options

	elements  map[string]*list.Element // lru releated elements
	evictList *list.List

	fn       interface{}
	fnKind   reflect.Kind
	fnNumIn  int
	fnNumOut int
	ttl      time.Duration
	items    map[string]*item
}

// New create a cache container with default capacity and given parameters.
func New(fn interface{}, ttl time.Duration, opt ...Option) (*Container, error) {
	return newContainer(fn, ttl, opt...)
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

func newContainer(fn interface{}, ttl time.Duration, opt ...Option) (*Container, error) {
	opts := &options{capacity: DefaultCapacity, cacheKeyGenerator: defaultCacheKeyGenerator}
	for _, o := range opt {
		o(opts)
	}
	if opts.capacity <= 0 {
		return nil, errors.New("capacity must be positive")
	}

	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func || t.NumOut() != 2 {
		return nil, ErrInvalidFn
	}

	c := &Container{
		opts:     opts,
		fn:       fn,
		fnKind:   t.Kind(),
		fnNumIn:  t.NumIn(),
		fnNumOut: t.NumOut(),
		ttl:      ttl,
	}
	if c.opts.enableLRU {
		c.evictList = list.New()
		c.elements = make(map[string]*list.Element)
	} else {
		c.items = make(map[string]*item)
	}
	return c, nil
}

type CacheKeyGenerator func(params ...interface{}) string

func defaultCacheKeyGenerator(params ...interface{}) string {
	// generate unique key
	buf := bytes.NewBufferString("")
	// FIXME: ["#" ""] and ["" "#"] will generate same key
	for _, param := range params {
		// convert pointer to reference value
		buf.WriteString(fmt.Sprintf("#%v", reflect.Indirect(reflect.ValueOf(param))))
	}
	return buf.String()
}

// Get is used to obtain the value with the given parameters. If the params string
// has in the container, it will return immediately. Otherwise, it will load data
// with the fn callback.
func (c *Container) Get(params ...interface{}) (interface{}, error) {
	// check params
	if len(params) != c.fnNumIn {
		return nil, ErrFnParams
	}

	key := c.opts.cacheKeyGenerator(params...)

	if !c.opts.enableLRU {
		if itm, ok := c.items[key]; ok {
			return itm.Value()
		}

		c.Lock()
		itm := c.getLocked(params, key)
		c.Unlock()

		return itm.Value()
	}

	if ent, ok := c.elements[key]; ok {
		c.Lock()
		c.evictList.MoveToFront(ent)
		c.Unlock()
		return ent.Value.(*item).Value()
	}

	c.Lock()
	ent := c.getLockedLRU(params, key)
	c.Unlock()

	return ent.Value.(*item).Value()

}

func (c *Container) getLocked(params []interface{}, key string) *item {
	if itm, ok := c.items[key]; ok {
		return itm
	}

	itm := newItem(params, key, c.ttl, c.fn)
	// copy on write
	items := make(map[string]*item, len(c.items)+1)
	for k, v := range c.items {
		items[k] = v
	}
	items[key] = itm
	c.items = items

	return itm
}

func (c *Container) getLockedLRU(params []interface{}, key string) *list.Element {
	if ent, ok := c.elements[key]; ok {
		c.evictList.MoveToFront(ent)
		return ent
	}

	itm := newItem(params, key, c.ttl, c.fn)
	ent := c.evictList.PushFront(itm)

	// copy on write
	elements := make(map[string]*list.Element, len(c.elements)+1)
	for k, v := range c.elements {
		elements[k] = v
	}
	elements[key] = ent
	c.elements = elements

	if c.evictList.Len() > c.opts.capacity {
		c.removeOldestElement()
	}

	return ent
}

// removeOldest removes the oldest item from the container.
func (c *Container) removeOldestElement() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the container.
func (c *Container) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	itm := e.Value.(*item)
	delete(c.elements, itm.key)
}

// Purge is used to completely clear the container
func (c *Container) Purge() {
	c.Lock()
	defer c.Unlock()
	if c.opts.enableLRU {
		for key := range c.elements {
			delete(c.elements, key)
		}
		c.evictList.Init()
	} else {
		for key := range c.items {
			delete(c.items, key)
		}
	}
}

// Remove removes the provided params from the container, returning if the
// params key was contained.
func (c *Container) Remove(params ...interface{}) bool {
	key := c.opts.cacheKeyGenerator(params...)
	c.Lock()
	defer c.Unlock()
	if c.opts.enableLRU {
		if ent, ok := c.elements[key]; ok {
			c.removeElement(ent)
			return true
		}
	} else {
		if _, ok := c.items[key]; ok {
			delete(c.items, key)
			return true
		}
	}
	return false
}

// Len returns the number of items in the container
func (c *Container) Len() int {
	c.RLock()
	defer c.RUnlock()
	if c.opts.enableLRU {
		return len(c.elements)
	}
	return len(c.items)
}

// item is used to hold a value
type item struct {
	key        string
	params     []interface{}
	value      interface{}
	err        error
	ttl        time.Duration
	expire     time.Time
	fn         interface{}
	initialed  bool
	initialCh  chan struct{}
	refreshing bool
	mu         sync.Mutex
}

// newItem constructs an item of the given parameters
func newItem(params []interface{}, key string, ttl time.Duration, fn interface{}) *item {
	return &item{
		key:       key,
		params:    params,
		ttl:       ttl,
		fn:        fn,
		initialCh: make(chan struct{}),
	}
}

// Value returns the real value in the item. If real value has been loaded,
// it will return immediately. Otherwise, it will return until the real value
// is initialed.
func (i *item) Value() (val interface{}, err error) {
	if time.Now().Before(i.expire) {
		return i.value, i.err
	}
	i.Refresh()
	// if item has not initialed, wait until initial done.
	// else return old value directly
	if !i.initialed {
		<-i.initialCh
	}
	return i.value, i.err
}

// Refresh is used to refresh real value with fn callback.
func (i *item) Refresh() {
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

func (i *item) refresh() {
	// load data with fn
	val, err := i.loadData()
	// don't cache response when underlying resouce exhausted
	if err != ErrResourceExhausted {
		i.value = val
		i.err = err
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
func (i *item) loadData() (interface{}, error) {
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
