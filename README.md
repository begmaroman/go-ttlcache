# go-ttlcache

go-ttlcache is an in-memory key:value store/cache similar to memcached that is
suitable for applications running on a single machine. Its major advantage is
that, being essentially a thread-safe and generically typed `map[K]V` with expiration
times, it doesn't need to serialize or transmit its contents over the network.

Comparable key with any object can be stored, for a given duration or forever, 
and the cache can be safely used by multiple goroutines.

Although go-ttlcache isn't meant to be used as a persistent datastore, the entire
cache can be saved to and loaded from a file (using `c.Items()` to retrieve the
items map to serialize, and `NewFrom()` to create a cache from a deserialized
one) to recover from downtime quickly. (See the docs for `NewFrom()` for caveats.)

### Installation

`go get github.com/begmaroman/go-ttlcache`

### Usage

```go
import (
	"fmt"
	"time"
	
    "github.com/begmaroman/go-ttlcache"
)

func main() {
	// Create a cache with a default expiration time of 5 minutes, and which
	// purges expired items every 10 minutes. Both key and value should be "string" type.
	c := ttlcache.New[string, string](5*time.Minute, 10*time.Minute)
	
	// There could be any comparable key type, and any value. For instance:
	// c := ttlcache.New[string, MyType](5*time.Minute, 10*time.Minute)

	// Set the value of the key "foo" to "bar", with the default expiration time
	c.Set("foo", "bar", ttlcache.DefaultExpiration)

	// Set the value of the key "baz" to "vaz", with no expiration time
	// (the item won't be removed until it is re-set, or removed using
	// c.Delete("baz")
	c.Set("baz", "vaz", ttlcache.NoExpiration)

	// Get the string associated with the key "foo" from the cache
	foo, found := c.Get("foo")
	if found {
		fmt.Println(foo)
	}

	// Since Go is statically typed, and cache values can be anything, type
	// assertion is needed when values are being passed to functions that don't
	// take arbitrary types, (i.e. interface{}). The simplest way to do this for
	// values which will only be used once--e.g. for passing to another
	// function--is:
	foo, found := c.Get("foo")
	if found {
		MyFunction(foo)
	}

	// This gets tedious if the value is used several times in the same function.
	// You might do either of the following instead:
	if foo, found := c.Get("foo"); found {
		// ...
	}
}
```

### Reference

`godoc` or [http://godoc.org/github.com/begmaroman/go-ttlcache](http://godoc.org/github.com/begmaroman/go-ttlcache)

Inspired by:
- https://github.com/jellydator/ttlcache
- https://github.com/patrickmn/go-cache