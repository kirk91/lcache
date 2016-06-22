package lcache

import (
	"sync"
	"time"
)

const (
	DefaultCapacity = 500
)

type Fn func() (interface{}, error)

type Item struct {
	value  interface{}
	ttl    time.Duration
	expire time.Time
	fn     Fn
}

func NewItem(ttl time.Duration, fn Fn) *Item {
	i := new(Item)
	i.ttl = ttl
	i.fn = fn
	i.expire = time.Now().Add(ttl)
	return i
}

// refresh use to refresh value with fn
func (i *Item) refresh() {
	if val, err := i.fn(); err == nil {
		i.value = val
	}
}

func (i *Item) Value() (val interface{}) {
	if time.Now().Before(i.expire) {
		return i.value
	} else {
		// refresh value with fn
		// only use one routine to refresh data
		i.refresh()
	}
	return i.value
}

type Container struct {
	sync.RWMutex
	size  int
	items map[string]*Item
}

func NewContainer() *Container {
	return &Container{
		size:  DefaultCapacity,
		items: make(map[string]*Item, DefaultCapacity),
	}
}

func (c *Container) Get(key string, ttl time.Duration, fn Fn) (val interface{}) {
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
