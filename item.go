package lcache

import (
	"fmt"
	"reflect"
	"sync"
	"time"
)

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
	return &Item{
		key:       fmt.Sprintf("%v", params),
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
