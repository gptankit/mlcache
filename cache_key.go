package mlcache

import "sync"

var pool *sync.Pool

func init() {

	// create new pool that returns CacheKey object
	pool = &sync.Pool{
		New: func() interface{} {
			return new(CacheKey)
		},
	}
}

type CacheKey struct {
	key string
}

// NewCacheKey takes a key and an expiry time and returns a cache key object.
func NewCacheKey(key string) *CacheKey {

	cacheKey := pool.Get().(*CacheKey)
	cacheKey.key = key

	return cacheKey
}

func (cacheKey *CacheKey) Done() {

	pool.Put(cacheKey)
}

func (cacheKey *CacheKey) AsString() string {
	return cacheKey.key
}
