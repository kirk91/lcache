package lcache

import (
	"sync"
	"testing"
	"time"
)

func TestContainer(t *testing.T) {
	var (
		c       *Container
		key     = "simple"
		val     interface{}
		start   time.Time
		cost    time.Duration
		expVal1         = "hello, world"
		expVal2         = "hello, pigger"
		retVal  *string = &expVal1
	)

	// after 100 millisecond, change return value
	time.AfterFunc(time.Millisecond*200, func() {
		retVal = &expVal2
	})

	c = NewContainer()
	fn1 := func() (interface{}, error) {
		// do something
		time.Sleep(50 * time.Millisecond)
		return *retVal, nil
	}

	start = time.Now()
	val = c.Get(key, 300*time.Millisecond, fn1)
	cost = time.Now().Sub(start)
	t.Logf("First call, cost: %v", cost)
	if val != expVal1 {
		t.Errorf("incorrect return value: %v", val)
	}
	if cost < time.Millisecond*50 {
		t.Errorf("cost time %v error", cost)
	}

	start = time.Now()
	val = c.Get(key, 300*time.Millisecond, fn1)
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
	val = c.Get(key, 300*time.Millisecond, fn1)
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
	val = c.Get(key, 300*time.Millisecond, fn1)
	cost = time.Now().Sub(start)
	t.Logf("Fourth call, cost: %v", cost)
	if val != expVal2 {
		t.Errorf("incorrect return value: %v", val)
	}
	if cost >= time.Millisecond*50 {
		t.Errorf("cost time %v error", cost)
	}
}

func TestRace(t *testing.T) {
	var (
		wg      sync.WaitGroup
		c       *Container
		expVal1         = "hello, world"
		expVal2         = "hello, pigger"
		retVal  *string = &expVal1
		key             = "race"
		ttl             = time.Nanosecond * 100
	)
	c = NewContainer()
	fn1 := func() (interface{}, error) {
		// do something
		time.Sleep(10 * time.Nanosecond)
		return *retVal, nil
	}
	c.Get(key, ttl, fn1)

	time.AfterFunc(time.Nanosecond*20, func() {
		retVal = &expVal2
	})
	repeat := 10000
	for i := 0; i < 3; i++ {
		wg.Add(repeat)
		go func() {
			var (
				start time.Time
				val   interface{}
			)
			for i := 0; i < repeat; i++ {
				start = time.Now()
				val = c.Get(key, ttl, fn1)
				t.Logf("get value: %v, cost: %v", val, time.Now().Sub(start))
				wg.Done()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkInitialRead(b *testing.B) {
	c := NewContainer()
	fn1 := func() (interface{}, error) {
		// do something
		time.Sleep(50 * time.Millisecond)
		return "hello, world", nil
	}
	key := "initialRead"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Get(key, 100*time.Millisecond, fn1)
		c.Remove(key)
	}
}

func BenchmarkInitialedRead(b *testing.B) {
	expVal1 := "hello, world"
	expVal2 := "hello, pigger"
	retVal := &expVal1
	c := NewContainer()
	fn1 := func() (interface{}, error) {
		// do something
		time.Sleep(50 * time.Millisecond)
		return *retVal, nil
	}
	key := "initialedRead"
	_ = c.Get(key, 1*time.Second, fn1)
	b.ResetTimer()
	time.AfterFunc(time.Millisecond*500, func() {
		retVal = &expVal2
	})
	for i := 0; i < b.N; i++ {
		_ = c.Get(key, 1*time.Second, fn1)
	}
}
