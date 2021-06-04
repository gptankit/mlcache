## Multi-Level Cache

**mlcache** is a multi-level cache interface with multiple *read* and *write* patterns supported. Some features - 

- Arranging cache implementations in a **multi-level cache format**.
- Option to choose from cache **read and write patterns**.
- Read patterns supported are **ReadThrough** and **CacheAside**.
- Write patterns supported are **WriteThrough**, **WriteAround** and **WriteBack**.
- Package defined **limit on number of cache levels**.
- User defined **limit on max size of cache value**.

### Import path

`import "github.com/gptankit/mlcache"`

### Typical usage

*mlcache* exposes a `Cacher` interface type that all caches must implement -

```
type Cacher interface {
	Get(key *CacheKey) (*bytes.Buffer, time.Time, error)
	Put(key *CacheKey, val *bytes.Buffer, expires time.Time) (CacheStatus, error)
	Del(key *CacheKey) (CacheStatus, error)
	IsPresent(key *CacheKey) (CacheStatus, error)
	Flush() error
}
```

Assuming `NewLnCacher()` is a user defined function that create a cache object satisfying `Cacher` interface and you have decided a read and write pattern combination that work best with your application setup, you can then go on to create the multi-level cacher and start using it - 

```
l1Cache, l2Cache, l3Cache := NewL1Cacher(), NewL2Cacher(), NewL3Cacher() // and so on...
cacher, mlcErr := mlcache.NewMultiLevelCache(mlcache.ReadThrough, mlcache.WriteThrough, 676, l1Cache, l2Cache, l3Cache)
if mlcErr != nil {
  // log error and panic
}

// create a key and a value
key := mlcache.NewCacheKey("my-key")
val := bytes.NewBuffer([]byte("my-val"))

// add to ml cache with selected write pattern
cacheStatus, putErr := cacher.Put(key, val, time.Now().UTC().Add(24*time.Hour))
if putErr != nil {
  // log error
}
if cacheStatus {
  // do your thing
}

// get from ml cache with selected read pattern
val, _, getErr := cacher.Get(key)
if getErr != nil {
  // log error
}
if val != nil {
  // do your thing
}
```

### Benchmarks (Get/Put)

Below are *mlcache* benchmarks with frequently used read/write patterns. All test cases include 3 levels of caching (L1,L2,L3) and store cache items in a map data structure with locking in place (R=Read, W=Write) - 

| BenchmarkCase | Iterations | TimePerIteration | 
| ------------- | ---------- | ---------------- |
| **ReadThrough/WriteThrough** ||
| BenchmarkCacher/R1-16 | 13849582    |            85.54 ns/op |
| BenchmarkCacher/W1-16 |   8077688   |            142.2 ns/op |
| **CacheAside/WriteThrough** ||
| BenchmarkCacher/R2-16 |   2278107   |            529.7 ns/op |
| BenchmarkCacher/W2-16 |   8070732   |            149.7 ns/op |
| **ReadThrough/WriteAround** ||
| BenchmarkCacher/R3-16 |  13211317   |             86.74 ns/op |
| BenchmarkCacher/W3-16 |  11639502   |            100.4 ns/op |
| **CacheAside/WriteAround** ||
| BenchmarkCacher/R4-16 |   2250987   |            528.0 ns/op |
| BenchmarkCacher/W4-16 |  12147741   |             95.82 ns/op |
| **ReadThrough/WriteBack** ||
| BenchmarkCacher/R5-16 |  12350166   |             85.95 ns/op |
| BenchmarkCacher/W5-16 |   2071774   |            691.1 ns/op |
| **CacheAside/WriteBack** ||
| BenchmarkCacher/R6-16 |   2448288   |            496.4 ns/op |
| BenchmarkCacher/W6-16 |   2102917   |            640.8 ns/op |

The numbers suggest that *ReadThrough/WriteThrough* and *ReadThrough/WriteAround* perform considerably better than other patterns under heavy load. Although, all patterns are useful under different circumstances and must be carefully studied and chosen depending on the needs of the application.
