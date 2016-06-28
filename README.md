# lcache
local cache container inspired by google [guava cahce](https://github.com/google/guava/wiki/CachesExplained)


## Design

- Concurrency-safety.
- If value not in cache, will load data synchronously.
- If value expired, will refresh data asynchronously. It will return old value until refreshed.
- Once refresh raise errors, the old value would be renewal.


## Usage

```go
import "lcache"

fn := func(x, y int, z string) (interface{}, error) {
    return "bar", nil
}
c := lcache.NewContainer(fn, time.Second * 60) // create cache container, default capicity is 512
// If cache data not in container, will add it in.
// The input params must be adapted with fn, else will return error.
// Cache key is string of params slice, e.g. "[0, 0, \"foo\"]"
val, err := c.Get(0, 0, "foo")
c.Remove(0, 0, "foo") // remove cache data with specified params
c.RemoveAll()  // remove all cache data
```

## Benchmark

- Golang 1.6.2
- OSX 10.11.3

```sh

# Read under key not in cache, it depends on the speed of data-loading
# In this benchmark case, load data cost 50 milliseconds
BenchmarkInitialRead-4  	      30	  51496897 ns/op


# Read under key in cache
# In this case, cached data may be expired
BenchmarkInitialedRead-4	 3000000	       390 ns/op
```

## Todo

- When the cache container is full, remove cache value by lru
