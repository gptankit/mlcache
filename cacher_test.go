package mlcache

import (
	"bytes"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gptankit/mlcache/errs"
)

type mockCacher struct {
	cache    map[string]*bytes.Buffer
	cacheMtx *sync.Mutex
}

func NewMockCacher() Cacher {

	cache := make(map[string]*bytes.Buffer)
	cacher := &mockCacher{
		cache:    cache,
		cacheMtx: &sync.Mutex{},
	}

	return cacher
}

func (c *mockCacher) Get(key *CacheKey) (*bytes.Buffer, time.Time, error) {

	c.cacheMtx.Lock()
	defer c.cacheMtx.Unlock()

	if val, ok := c.cache[key.AsString()]; ok {
		return val, time.Now().UTC(), nil
	} else {
		return nil, time.Now().UTC(), errs.New("Could not get item")
	}
}

func (c *mockCacher) Put(key *CacheKey, val *bytes.Buffer, expires time.Time) (CacheStatus, error) {

	c.cacheMtx.Lock()
	defer c.cacheMtx.Unlock()

	c.cache[key.AsString()] = val
	return CacheStatusSuccess, nil
}

func (c *mockCacher) Del(key *CacheKey) (CacheStatus, error) {

	c.cacheMtx.Lock()
	defer c.cacheMtx.Unlock()

	delete(c.cache, key.AsString())
	return CacheStatusSuccess, nil
}

func (c *mockCacher) IsPresent(key *CacheKey) (CacheStatus, error) {

	c.cacheMtx.Lock()
	defer c.cacheMtx.Unlock()

	if _, ok := c.cache[key.AsString()]; ok {
		return CacheStatusSuccess, nil
	} else {
		return CacheStatusFailure, nil
	}
}

func (c *mockCacher) Flush() error {

	c.cacheMtx.Lock()
	defer c.cacheMtx.Unlock()

	c.cache = make(map[string]*bytes.Buffer)
	return nil
}

type test struct {
	id           uint8
	readPattern  ReadPattern
	writePattern WritePattern
	maxValSize   int
	ttl          time.Duration
	cacheKey     string
	cacheVal     string
	wantEmpty    bool
	wantInL1     bool
	wantInL2     bool
	wantInL3     bool
	assert       func(string, string) bool
}

func TestCacher(t *testing.T) {

	tests := createTestCases()

	for _, test := range tests {

		// multi-level cacher with 3 levels of cache
		l1Cache, l2Cache, l3Cache := NewMockCacher(), NewMockCacher(), NewMockCacher()
		cacher, err := NewMultiLevelCache(test.readPattern, test.writePattern, test.maxValSize, l1Cache, l2Cache, l3Cache)
		if err != nil {
			t.Errorf("testid:%d: problem creating cacher\n", test.id)
			t.FailNow()
		}

		key := NewCacheKey(test.cacheKey)
		val := bytes.NewBuffer([]byte(test.cacheVal))

		// add to ml cache with selected write pattern
		cacher.Put(key, val, time.Now().Add(test.ttl))

		val, _, _ = l1Cache.Get(key)
		if test.wantInL1 == reflect.ValueOf(val).IsNil() {
			t.Errorf("testid:%d: wantInL1 %v did not succeed\n", test.id, test.wantInL1)
			t.FailNow()
		}
		val, _, _ = l2Cache.Get(key)
		if test.wantInL2 == reflect.ValueOf(val).IsNil() {
			t.Errorf("testid:%d: wantInL2 %v did not succeed\n", test.id, test.wantInL2)
			t.FailNow()
		}
		val, _, _ = l3Cache.Get(key)
		if test.wantInL3 == reflect.ValueOf(val).IsNil() {
			t.Errorf("testid:%d: wantInL3 %v did not succeed\n", test.id, test.wantInL3)
			t.FailNow()
		}

		// get from ml cache with selected read pattern
		val, _, _ = cacher.Get(key)

		if test.wantEmpty != reflect.ValueOf(val).IsNil() {
			t.Errorf("testid:%d: wantEmpty %v did not succeed\n", test.id, test.wantEmpty)
			t.FailNow()
		}

		if val != nil && !test.assert(val.String(), test.cacheVal) {
			t.Errorf("testid:%d: want %s, got %s  \n", test.id, test.cacheVal, val.String())
			t.FailNow()
		}

		key.Done()
		cacher.Flush()
	}
}

func BenchmarkCacher(b *testing.B) {

	tests := createTestCases()

	for _, test := range tests {

		// multi-level cacher with 3 levels of cache
		l1Cache, l2Cache, l3Cache := NewMockCacher(), NewMockCacher(), NewMockCacher()
		cacher, err := NewMultiLevelCache(test.readPattern, test.writePattern, test.maxValSize, l1Cache, l2Cache, l3Cache)
		if err != nil {
			b.Errorf("testid:%d: problem creating cacher\n", test.id)
			b.FailNow()
		}

		key := NewCacheKey(test.cacheKey)
		val := bytes.NewBuffer([]byte(test.cacheVal))

		testid := strconv.Itoa(int(test.id))
		b.Run("W"+testid, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// add to ml cache with selected write pattern
				cacher.Put(key, val, time.Now().Add(test.ttl))
			}
		})

		b.Run("R"+testid, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// get from ml cache with selected read pattern
				cacher.Get(key)
			}
		})

		key.Done()
		cacher.Flush()
	}
}

func createTestCases() []test {

	return []test{
		{
			id:           1,
			readPattern:  ReadThrough,
			writePattern: WriteThrough,
			maxValSize:   100,
			ttl:          5 * time.Second,
			cacheKey:     "metamorphosis",
			cacheVal:     "franzkafka",
			wantEmpty:    false,
			wantInL1:     true,
			wantInL2:     true,
			wantInL3:     true,
			assert: func(s1 string, s2 string) bool {
				return s1 == s2
			},
		},
		{
			id:           2,
			readPattern:  CacheAside,
			writePattern: WriteThrough,
			maxValSize:   100,
			ttl:          5 * time.Second,
			cacheKey:     "thecatcherintherye",
			cacheVal:     "jdsalinger",
			wantEmpty:    false,
			wantInL1:     true,
			wantInL2:     true,
			wantInL3:     true,
			assert: func(s1 string, s2 string) bool {
				return s1 == s2
			},
		},
		{
			id:           3,
			readPattern:  ReadThrough,
			writePattern: WriteAround,
			maxValSize:   100,
			ttl:          5 * time.Second,
			cacheKey:     "cosmos",
			cacheVal:     "carlsagan",
			wantEmpty:    false,
			wantInL1:     false,
			wantInL2:     false,
			wantInL3:     true,
			assert: func(s1 string, s2 string) bool {
				return s1 == s2
			},
		},
		{
			id:           4,
			readPattern:  CacheAside,
			writePattern: WriteAround,
			maxValSize:   100,
			ttl:          5 * time.Second,
			cacheKey:     "siddhartha",
			cacheVal:     "hermannhesse",
			wantEmpty:    false,
			wantInL1:     false,
			wantInL2:     false,
			wantInL3:     true,
			assert: func(s1 string, s2 string) bool {
				return s1 == s2
			},
		},
		{
			id:           5,
			readPattern:  ReadThrough,
			writePattern: WriteBack,
			maxValSize:   100,
			ttl:          5 * time.Second,
			cacheKey:     "thesirensoftitan",
			cacheVal:     "kurtvonnegut",
			wantEmpty:    false,
			wantInL1:     true,
			wantInL2:     false,
			wantInL3:     false,
			assert: func(s1 string, s2 string) bool {
				return s1 == s2
			},
		},
		{
			id:           6,
			readPattern:  CacheAside,
			writePattern: WriteBack,
			maxValSize:   100,
			ttl:          5 * time.Second,
			cacheKey:     "thecolorpurple",
			cacheVal:     "alicewalker",
			wantEmpty:    false,
			wantInL1:     true,
			wantInL2:     false,
			wantInL3:     false,
			assert: func(s1 string, s2 string) bool {
				return s1 == s2
			},
		},
	}
}
