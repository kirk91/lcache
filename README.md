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

c := lcache.NewContainer() // create cache container
fn := func() (interface{}, error) {
    return "bar", nil
}
ttl := time.Second * 30  // 30 senconds ttl
// If cache data not in container, will add it in.
// And return cached value
val := c.Get("foo", ttl, fn)
c.Remove("foo") // remove cache data with specified key
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
BenchmarkInitialedRead-4	20000000	       104 ns/op
```

## Todo

- When the cache container is full, remove cache value by lru
