package lcache

import (
	"sync"
	"time"
)

const (
	DefaultCapacity = 128
)

type Fn func() (interface{}, error)

type Item struct {
	value      interface{}
	expire     time.Time
	fn         Fn
	ttl        time.Duration
	initialed  bool
	initialCh  chan struct{}
	refreshing bool
	mu         sync.Mutex
}

func NewItem(ttl time.Duration, fn Fn) *Item {
	return &Item{
		ttl:       ttl,
		fn:        fn,
		initialCh: make(chan struct{}),
	}
}

func (i *Item) Value() (val interface{}) {
	if time.Now().Before(i.expire) {
		return i.value
	} else {
		i.Refresh()
		// if item has not initialed, wait until initial done.
		// else return old value directly
		if !i.initialed {
			<-i.initialCh
		}
	}
	return i.value
}

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
	if val, err := i.fn(); err == nil {
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

type Container struct {
	sync.RWMutex
	capacity int
	items    map[string]*Item
}

func NewContainer() *Container {
	c := new(Container)
	c.capacity = DefaultCapacity
	c.items = make(map[string]*Item, c.capacity)
	return c
}

func (c *Container) Add(key string, ttl time.Duration, fn Fn) {
	c.Lock()
	if _, ok := c.items[key]; !ok {
		c.items[key] = NewItem(ttl, fn)
	}
	c.Unlock()
}

func (c *Container) Get(key string, ttl time.Duration, fn Fn) interface{} {
	var (
		item *Item
		ok   bool
	)
	c.Lock()
	if item, ok = c.items[key]; !ok {
		item = NewItem(ttl, fn)
		c.items[key] = item
	}
	c.Unlock()
	return item.Value()
}

func (c *Container) Remove(key string) {
	c.Lock()
	delete(c.items, key)
	c.Unlock()
}

func (c *Container) RemoveAll() {
	// cow
	newItems := make(map[string]*Item, c.capacity)
	c.items = newItems
}
