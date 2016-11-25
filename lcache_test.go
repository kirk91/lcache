package lcache

import (
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
		expVal2 = "hello, pigger"
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

func TestEvictContainer(t *testing.T) {
	fn := func(x, y int) (interface{}, error) {
		return "hello, world", nil
	}
	c, _ := NewWithSize(2, fn, 300*time.Millisecond)

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
