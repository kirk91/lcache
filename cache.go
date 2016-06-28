package lcache

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

const (
	DefaultCapacity = 512
)

var (
	ErrInvalidFn = errors.New("invalid cache fn")
	ErrFnParams  = errors.New("cache fn params not adapted")
)

type Container struct {
	sync.RWMutex
	capacity int
	fn       interface{}
	fnNumIn  int
	ttl      time.Duration
	items    map[string]*Item
}

func NewContainer(fn interface{}, ttl time.Duration) (*Container, error) {
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func {
		return nil, ErrInvalidFn
	}
	if t.NumOut() != 2 {
		return nil, ErrInvalidFn
	}
	c := new(Container)
	c.capacity = DefaultCapacity
	c.fn = fn
	c.fnNumIn = t.NumIn()
	c.ttl = ttl
	c.items = make(map[string]*Item, c.capacity)
	return c, nil
}

func (c *Container) Get(params ...interface{}) (interface{}, error) {
	if len(params) != c.fnNumIn {
		return nil, ErrFnParams
	}
	var (
		item *Item
		ok   bool
	)
	key := fmt.Sprintf("%v", params)
	c.Lock()
	if item, ok = c.items[key]; !ok {
		item = NewItem(params, c.ttl, c.fn)
		c.items[key] = item
	}
	c.Unlock()
	return item.Value(), nil
}

func (c *Container) RemoveAll() {
	c.items = make(map[string]*Item, c.capacity)
}

func (c *Container) Remove(params ...interface{}) {
	key := fmt.Sprintf("%v", params)
	c.Lock()
	delete(c.items, key)
	c.Unlock()
}

func (c *Container) Count() int {
	return len(c.items)
}

type Item struct {
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

func NewItem(params []interface{}, ttl time.Duration, fn interface{}) *Item {
	return &Item{
		params:    params,
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
