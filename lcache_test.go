package lcache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestContainer(t *testing.T) {
	var (
		c *Container
		// key     = "simple"
		val     interface{}
		start   time.Time
		cost    time.Duration
		expVal1 = "hello, world"
		expVal2 = "hello, kirk"
		retVal  = &expVal1
	)

	// after 100 millisecond, change return value
	time.AfterFunc(time.Millisecond*200, func() {
		retVal = &expVal2
	})

	fn1 := func(x, y int) (interface{}, error) {
		// do something
		time.Sleep(50 * time.Millisecond)
		return *retVal, nil
	}
	c, _ = New(fn1, 300*time.Millisecond)

	start = time.Now()
	val, _ = c.Get(1, 2)
	cost = time.Now().Sub(start)
	t.Logf("First call, cost: %v", cost)
	if val != expVal1 {
		t.Errorf("incorrect return value: %v", val)
	}
	if cost < time.Millisecond*50 {
		t.Errorf("cost time %v error", cost)
	}

	start = time.Now()
	val, _ = c.Get(1, 2)
	cost = time.Now().Sub(start)
	t.Logf("Second call, cost: %v", cost)
	if cost >= time.Millisecond*50 {
		t.Errorf("cost time %v error", cost)
	}
	if val != expVal1 {
		t.Errorf("incorrect return value: %v", val)
	}

	// after 300 milliseconds, read will refresh cache asynchronously
	time.Sleep(300 * time.Millisecond)
	start = time.Now()
	val, _ = c.Get(1, 2)
	cost = time.Now().Sub(start)
	t.Logf("Third call, cost: %v", cost)
	if val != expVal1 {
		t.Errorf("incorrect return value: %v", val)
	}
	if cost >= time.Millisecond*50 {
		t.Errorf("cost time %v error", cost)
	}

	// two seconds later, will read new cache value
	time.Sleep(100 * time.Millisecond)
	start = time.Now()
	val, _ = c.Get(1, 2)
	cost = time.Now().Sub(start)
	t.Logf("Fourth call, cost: %v", cost)
	if val != expVal2 {
		t.Errorf("incorrect return value: %v", val)
	}
	if cost >= time.Millisecond*50 {
		t.Errorf("cost time %v error", cost)
	}
}

func TestConcurrency(t *testing.T) {
	fn := func(x, y int) (interface{}, error) {
		return x + y, nil
	}
	c, _ := New(fn, time.Second*10)
	ch := make(chan struct{})
	startCh := make(chan struct{})
	go func() {
		<-startCh
		for {
			select {
			case <-ch:
				return
			default:
				c.Get(1, rand.Intn(100))
			}
		}
	}()
	go func() {
		<-startCh
		for {
			select {
			case <-ch:
				return
			default:
				c.Get(1, rand.Intn(100))
			}
		}
	}()
	go func() {
		<-startCh
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("unexpected panic: %v", r)
			}
		}()
		for {
			select {
			case <-ch:
				return
			default:
				c.Get(1, rand.Intn(100))
			}
		}
	}()
	close(startCh)
	time.Sleep(1 * time.Second)
	close(ch)
}

func TestError(t *testing.T) {
	expectedErr := errors.New("expected error")
	retErr := expectedErr
	fn := func(x, y int) (interface{}, error) {
		return nil, retErr
	}
	c, _ := New(fn, time.Millisecond*30)
	_, err := c.Get(1, 2)
	if err != expectedErr {
		t.Errorf("expected error: %v, but got: %v", expectedErr, err)
	}
	retErr = ErrResourceExhausted
	time.Sleep(time.Millisecond * 40)
	if err != expectedErr {
		t.Errorf("expected error: %v, but got: %v", expectedErr, err)
	}
}

func TestEvictContainer(t *testing.T) {
	fn := func(x, y int) (interface{}, error) {
		return "hello, world", nil
	}
	c, _ := New(fn, 300*time.Millisecond, WithCapacity(2), WithLRU())

	// first
	c.Get(1, 2)
	if c.Len() != 1 {
		t.Errorf("container expected length is 1, but got %d", c.Len())
	}

	// second
	c.Get(2, 3)
	if c.Len() != 2 {
		t.Errorf("container expected length is 2, but got %d", c.Len())
	}

	// third
	c.Get(3, 4)
	if c.Len() != 2 {
		t.Errorf("container expected length is 2, but got %d", c.Len())
	}
}

type Foo struct {
	A int
	B string
}

func TestPointerValues(t *testing.T) {
	fn := func(f *Foo) (interface{}, error) {
		return nil, nil
	}
	c, _ := New(fn, 300*time.Millisecond, WithCapacity(2))

	c.Get(new(Foo))
	c.Get(new(Foo))
	if c.Len() != 1 {
		t.Errorf("container expected length is 1, but got %d", c.Len())
	}
	c.Remove(new(Foo))
	if c.Len() != 0 {
		t.Errorf("container expected length is 0, but got %d", c.Len())
	}
}

func TestMust(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("expected nil, but got panic: %s", ErrInvalidFn)
		}
	}()
	Must(New(func() (interface{}, error) { return nil, nil }, time.Nanosecond))
}

func TestMustPanic(t *testing.T) {
	defer func() {
		if p := recover(); p == nil {
			t.Errorf("expected panic: %s, but got nil", ErrInvalidFn)
		}
	}()
	Must(New("123", time.Nanosecond))
}

func TestRace(t *testing.T) {
	var (
		wg      sync.WaitGroup
		c       *Container
		expVal1         = "hello, world"
		expVal2         = "hello, pigger"
		retVal  *string = &expVal1
		// key             = "race"
		ttl = time.Nanosecond * 100
	)
	fn1 := func() (interface{}, error) {
		// do something
		time.Sleep(10 * time.Nanosecond)
		return *retVal, nil
	}
	c, _ = New(fn1, ttl)
	c.Get()

	time.AfterFunc(time.Nanosecond*20, func() {
		retVal = &expVal2
	})
	repeat := 10000
	for i := 0; i < 3; i++ {
		wg.Add(repeat)
		go func() {
			for i := 0; i < repeat; i++ {
				c.Get()
				wg.Done()
			}
		}()
	}
	wg.Wait()
}

func TestCustomCacheKeyGenerator(t *testing.T) {
	fn := func(x, y int) (interface{}, error) {
		return x + y + int(time.Now().UnixNano()), nil
	}
	g := func(params ...interface{}) string {
		// generate unique key
		buf := bytes.NewBufferString("")
		// FIXME: ["#" ""] and ["" "#"] will generate same key
		for _, param := range params {
			// convert pointer to reference value
			buf.WriteString(fmt.Sprintf("^%v", reflect.Indirect(reflect.ValueOf(param))))
		}
		return buf.String()
	}
	c, _ := New(fn, time.Second*5, WithCacheKeyGenerator(g))
	res, _ := c.Get(1, 2)
	res1, _ := c.Get(1, 2)
	if res != res1 {
		t.Errorf("expected %v, but got %v", res, res1)
	}
}

func TestContextSupport(t *testing.T) {
	fn := func(ctx context.Context, x, y int) (interface{}, error) {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		default:
			return x + y + int(time.Now().UnixNano()), nil
		}
	}
	c, _ := New(fn, time.Millisecond*100, WithContextSupport())
	ctx, cancel := context.WithCancel(context.Background())

	// different context instance
	res, _ := c.Get(ctx, 1, 2)
	res1, _ := c.Get(context.Background(), 1, 2)
	if res != res1 {
		t.Fatalf("expected %v, but got %v", res, res1)
	}

	// context canceled
	cancel()
	time.Sleep(time.Millisecond * 150)
	c.Get(ctx, 1, 2)                  // trigger async refresh
	time.Sleep(time.Millisecond * 50) // waiting async refresh
	_, err := c.Get(ctx, 1, 2)
	if err == context.Canceled {
		t.Errorf("unexpected context canceled")
	}
}

func BenchmarkInitialRead(b *testing.B) {
	fn1 := func() (interface{}, error) {
		// do something
		time.Sleep(50 * time.Millisecond)
		return "hello, world", nil
	}
	c, _ := New(fn1, 100*time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get()
		c.Remove()
	}
}

func BenchmarkInitialedRead(b *testing.B) {
	expVal1 := "hello, world"
	expVal2 := "hello, pigger"
	retVal := &expVal1
	fn1 := func() (interface{}, error) {
		// do something
		time.Sleep(50 * time.Millisecond)
		return *retVal, nil
	}
	c, _ := New(fn1, 1*time.Second)
	c.Get()
	b.ResetTimer()
	time.AfterFunc(time.Millisecond*500, func() {
		retVal = &expVal2
	})
	for i := 0; i < b.N; i++ {
		c.Get()
	}
}
