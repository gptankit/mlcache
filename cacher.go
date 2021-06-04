package mlcache

import (
	"bytes"
	"math"
	"time"

	"github.com/gptankit/mlcache/errs"
)

var _ Cacher = &cacher{}

type CacheStatus bool

const (
	CacheStatusSuccess CacheStatus = true
	CacheStatusFailure CacheStatus = false
)

const (
	NoWorkableCacheFound  errs.ErrorMessage = "No workable cache found"
	MaxCacheLevelExceeded errs.ErrorMessage = "Max cache level exceeded"
	MaxValLenExceeded     errs.ErrorMessage = "Max val len exceeded"
	GetError              errs.ErrorMessage = "Get error"
	PutError              errs.ErrorMessage = "Put error"
	DelError              errs.ErrorMessage = "Del error"
	IsPresentError        errs.ErrorMessage = "IsPresent error"
	FlushError            errs.ErrorMessage = "Flush error"
)

type ReadPattern uint8
type WritePattern uint8

const (
	ReadThrough ReadPattern = iota
	CacheAside
)

const (
	WriteThrough WritePattern = iota
	WriteAround
	WriteBack
)

const maxCaches uint8 = 5
const MaxCacheValSize int = math.MaxInt32

// Cacher is an interface for cache implementers
type Cacher interface {
	// Get returns the cache item, if present
	Get(key *CacheKey) (*bytes.Buffer, time.Time, error)
	// Put adds/updates a cache item
	Put(key *CacheKey, val *bytes.Buffer, expires time.Time) (CacheStatus, error)
	// Del deletes the key from the cache
	Del(key *CacheKey) (CacheStatus, error)
	// IsPresent returns true if the key is present
	IsPresent(key *CacheKey) (CacheStatus, error)
	// Flush clears all keys from the cache
	Flush() error
}

type lCache struct {
	cur  Cacher
	prev *lCache
	next *lCache
}

type cacher struct {
	l1Cache      *lCache
	lnCache      *lCache
	numCaches    uint8
	readPattern  ReadPattern
	writePattern WritePattern
	maxValSize   int
}

func NewMultiLevelCache(readPattern ReadPattern, writePattern WritePattern, maxValSize int, caches ...Cacher) (Cacher, error) {

	numCaches := uint8(len(caches))

	if numCaches == 0 { // if called with no cache parameters
		return nil, errs.New(NoWorkableCacheFound)
	} else if numCaches > maxCaches { // if called with more than maxCache limit
		return nil, errs.New(MaxCacheLevelExceeded)
	}

	ci := 0
	eCache := &lCache{cur: caches[ci], prev: nil, next: nil}
	sCache := eCache // save head
	ci++
	for ci < int(numCaches) {
		eCache.next = &lCache{cur: caches[ci], prev: eCache, next: nil}
		eCache = eCache.next
		ci++
	}

	ca := &cacher{
		l1Cache:      sCache,
		lnCache:      eCache,
		numCaches:    numCaches,
		readPattern:  readPattern,
		writePattern: writePattern,
		maxValSize:   maxValSize,
	}

	return ca, nil
}

func (ca *cacher) Get(key *CacheKey) (*bytes.Buffer, time.Time, error) {

	if key == nil {
		return nil, time.Now().UTC(), errs.New(GetError)
	}

	switch ca.readPattern {
	case ReadThrough:
		this := ca.l1Cache
		for this != nil {
			cache := this.cur
			val, ttl, _ := cache.Get(key) // lookup in a cache
			if val != nil {               // if found in higher level cache, sync populate all lower level caches and return from lowest level cache
				lower := this.prev
				for lower != nil {
					status, err := lower.cur.Put(key, val, ttl)
					if err != nil || status == CacheStatusFailure {
						return nil, time.Now().UTC(), err
					}
					lower = lower.prev
				}
				return val, ttl, nil
			} else {
				this = this.next
			}
		}

	case CacheAside:
		this := ca.l1Cache
		for this != nil {
			cache := this.cur
			val, ttl, _ := cache.Get(key) // lookup in a cache
			if val != nil {               // if found in higher level cache, return first and async populate all lower level caches
				go func() {
					lower := this.prev
					for lower != nil {
						lower.cur.Put(key, val, ttl)
						lower = lower.prev
					}
				}()
				return val, ttl, nil
			} else {
				this = this.next
			}
		}
	}

	return nil, time.Now().UTC(), nil
}

func (ca *cacher) Put(key *CacheKey, val *bytes.Buffer, ttl time.Time) (CacheStatus, error) {

	if key == nil {
		return CacheStatusFailure, errs.New(PutError)
	}

	if val != nil && val.Len() > ca.maxValSize {
		return CacheStatusFailure, errs.New(MaxValLenExceeded)
	}

	switch ca.writePattern {
	case WriteThrough:
		this := ca.l1Cache
		for this != nil {
			cache := this.cur
			cacheStatus, err := cache.Put(key, val, ttl) // put in all cache levels
			if err != nil || cacheStatus == CacheStatusFailure {
				return CacheStatusFailure, errs.Build(err, PutError)
			}
			this = this.next
		}
		return CacheStatusSuccess, nil

	case WriteAround:
		this := ca.lnCache
		cache := this.cur                            // get level n cache
		cacheStatus, err := cache.Put(key, val, ttl) // put in a cache
		if err != nil || cacheStatus == CacheStatusFailure {
			return CacheStatusFailure, errs.Build(err, PutError)
		}
		return CacheStatusSuccess, nil

	case WriteBack:
		this := ca.l1Cache
		cache := this.cur                            // get level 1 cache
		cacheStatus, err := cache.Put(key, val, ttl) // put in a cache
		if err != nil || cacheStatus == CacheStatusFailure {
			return CacheStatusFailure, errs.Build(err, PutError)
		} else {
			go func(writeBackDelay time.Duration) { // put in upper level caches with delay
				time.Sleep(writeBackDelay)

				upper := this.next
				for upper != nil {
					status, err := upper.cur.Put(key, val, ttl)
					if err != nil || status == CacheStatusFailure {
						break
					}
					upper = upper.next
				}
			}(200 * time.Millisecond)
		}
		return CacheStatusSuccess, nil
	}

	return CacheStatusSuccess, nil
}

func (ca *cacher) Del(key *CacheKey) (CacheStatus, error) {

	if key == nil {
		return CacheStatusFailure, errs.New(DelError)
	}

	// deleting order -> level n to level 1
	this := ca.lnCache
	for this != nil {
		cache := this.cur
		cacheStatus, err := cache.Del(key) // delete from cache
		if err != nil || cacheStatus == CacheStatusFailure {
			return CacheStatusFailure, errs.Build(err, DelError)
		}
		this = this.prev
	}

	return CacheStatusSuccess, nil
}

func (ca *cacher) IsPresent(key *CacheKey) (CacheStatus, error) {

	if key == nil {
		return CacheStatusFailure, errs.New(IsPresentError)
	}

	this := ca.l1Cache
	cache := this.cur                        // get level 1 cache
	cacheStatus, err := cache.IsPresent(key) // check only in level 1 cache, assuming all caches are in sync
	if err != nil || cacheStatus == CacheStatusFailure {
		return CacheStatusFailure, errs.Build(err, IsPresentError)
	}

	return CacheStatusSuccess, nil
}

func (ca *cacher) Flush() error {

	this := ca.l1Cache
	for this != nil {
		cache := this.cur
		go cache.Flush() // async flush
		this = this.next
	}
	return nil
}
