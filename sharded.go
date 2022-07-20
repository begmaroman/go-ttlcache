package ttlcache

import (
	"crypto/rand"
	"math"
	"math/big"
	insecurerand "math/rand"
	"os"
	"runtime"
	"time"
)

// This is an experimental and unexported (for now) attempt at making a cache
// with better algorithmic complexity than the standard one, namely by
// preventing write locks of the entire cache when an item is added. As of the
// time of writing, the overhead of selecting buckets results in cache
// operations being about twice as slow as for the standard cache with small
// total cache sizes, and faster for larger ones.
//
// See cache_test.go for a few benchmarks.

type unexportedShardedCache[K comparable, V any] struct {
	*shardedCache[K, V]
}

type shardedCache[K comparable, V any] struct {
	seed    uint32
	m       uint32
	cs      []*cache[K, V]
	janitor *shardedJanitor[K, V]
}

// djb2 with better shuffling. 5x faster than FNV with the hash.Hash overhead.
func djb33[K comparable, V any](seed uint32, k K) uint32 {
	var kRaw []byte
	switch key := any(k).(type) {
	case string:
		kRaw = []byte(key)
	case []byte:
		kRaw = key
	}

	var (
		l = uint32(len(kRaw))
		d = 5381 + seed + l
		i = uint32(0)
	)
	// Why is all this 5x faster than a for loop?
	if l >= 4 {
		for i < l-4 {
			d = (d * 33) ^ uint32(kRaw[i])
			d = (d * 33) ^ uint32(kRaw[i+1])
			d = (d * 33) ^ uint32(kRaw[i+2])
			d = (d * 33) ^ uint32(kRaw[i+3])
			i += 4
		}
	}
	switch l - i {
	case 1:
	case 2:
		d = (d * 33) ^ uint32(kRaw[i])
	case 3:
		d = (d * 33) ^ uint32(kRaw[i])
		d = (d * 33) ^ uint32(kRaw[i+1])
	case 4:
		d = (d * 33) ^ uint32(kRaw[i])
		d = (d * 33) ^ uint32(kRaw[i+1])
		d = (d * 33) ^ uint32(kRaw[i+2])
	}
	return d ^ (d >> 16)
}

func (sc *shardedCache[K, V]) bucket(k K) *cache[K, V] {
	return sc.cs[djb33[K, V](sc.seed, k)%sc.m]
}

func (sc *shardedCache[K, V]) Set(k K, x V, d time.Duration) {
	sc.bucket(k).Set(k, x, d)
}

func (sc *shardedCache[K, V]) Add(k K, x V, d time.Duration) error {
	return sc.bucket(k).Add(k, x, d)
}

func (sc *shardedCache[K, V]) Replace(k K, x V, d time.Duration) error {
	return sc.bucket(k).Replace(k, x, d)
}

func (sc *shardedCache[K, V]) Get(k K) (interface{}, bool) {
	return sc.bucket(k).Get(k)
}

func (sc *shardedCache[K, V]) Delete(k K) {
	sc.bucket(k).Delete(k)
}

func (sc *shardedCache[K, V]) DeleteExpired() {
	for _, v := range sc.cs {
		v.DeleteExpired()
	}
}

// Returns the items in the cache. This may include items that have expired,
// but have not yet been cleaned up. If this is significant, the Expiration
// fields of the items should be checked. Note that explicit synchronization
// is needed to use a cache and its corresponding Items() return values at
// the same time, as the maps are shared.
func (sc *shardedCache[K, V]) Items() []map[K]Item[V] {
	res := make([]map[K]Item[V], len(sc.cs))
	for i, v := range sc.cs {
		res[i] = v.Items()
	}
	return res
}

func (sc *shardedCache[K, V]) Flush() {
	for _, v := range sc.cs {
		v.Flush()
	}
}

type shardedJanitor[K comparable, V any] struct {
	Interval time.Duration
	stop     chan bool
}

func (j *shardedJanitor[K, V]) Run(sc *shardedCache[K, V]) {
	j.stop = make(chan bool)
	tick := time.Tick(j.Interval)
	for {
		select {
		case <-tick:
			sc.DeleteExpired()
		case <-j.stop:
			return
		}
	}
}

func stopShardedJanitor[K comparable, V any](sc *unexportedShardedCache[K, V]) {
	sc.janitor.stop <- true
}

func runShardedJanitor[K comparable, V any](sc *shardedCache[K, V], ci time.Duration) {
	j := &shardedJanitor[K, V]{
		Interval: ci,
	}
	sc.janitor = j
	go j.Run(sc)
}

func newShardedCache[K comparable, V any](n int, de time.Duration) *shardedCache[K, V] {
	max := big.NewInt(0).SetUint64(uint64(math.MaxUint32))
	rnd, err := rand.Int(rand.Reader, max)
	var seed uint32
	if err != nil {
		os.Stderr.Write([]byte("WARNING: go-cache's newShardedCache failed to read from the system CSPRNG (/dev/urandom or equivalent.) Your system's security may be compromised. Continuing with an insecure seed.\n"))
		seed = insecurerand.Uint32()
	} else {
		seed = uint32(rnd.Uint64())
	}
	sc := &shardedCache[K, V]{
		seed: seed,
		m:    uint32(n),
		cs:   make([]*cache[K, V], n),
	}
	for i := 0; i < n; i++ {
		c := &cache[K, V]{
			defaultExpiration: de,
			items:             map[K]Item[V]{},
		}
		sc.cs[i] = c
	}
	return sc
}

func unexportedNewSharded[K comparable, V any](defaultExpiration, cleanupInterval time.Duration, shards int) *unexportedShardedCache[K, V] {
	if defaultExpiration == 0 {
		defaultExpiration = -1
	}
	sc := newShardedCache[K, V](shards, defaultExpiration)
	SC := &unexportedShardedCache[K, V]{sc}
	if cleanupInterval > 0 {
		runShardedJanitor(sc, cleanupInterval)
		runtime.SetFinalizer(SC, stopShardedJanitor[K, V])
	}
	return SC
}
