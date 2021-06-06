package mlcache

import (
	"bytes"
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
	InvalidReadPattern    errs.ErrorMessage = "Invalid read pattern"
	InvalidWritePattern   errs.ErrorMessage = "Invalid write pattern"
	GetError              errs.ErrorMessage = "Get error"
	PutError              errs.ErrorMessage = "Put error"
	DelError              errs.ErrorMessage = "Del error"
	IsPresentError        errs.ErrorMessage = "IsPresent error"
	FlushError            errs.ErrorMessage = "Flush error"
)

type ReadPattern uint8
type WritePattern uint8

// List of read patterns implemented
const (
	// Reads from all caches but backfills first
	ReadThrough ReadPattern = iota
	// Reads from all caches but backfills later
	CacheAside
	// End marker, no biggie
	endR
)

// List of write patterns implemented
const (
	// Writes to all caches in a linear fashion
	WriteThrough WritePattern = iota
	// Writes to the last cache only
	WriteAround
	// Writes to the first cache while writing to other caches with slight delay
	WriteBack
	// End marker, no biggie
	endW
)

// Max number of cahe levels allowed in the system
const maxCaches uint8 = 5

// Cacher is an interface to be used by concrete cache implementation
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

// lCache is a doubly linked list of caches in the system
type lCache struct {
	cur  Cacher
	prev *lCache
	next *lCache
}

// cacher is multi-level cache object
type cacher struct {
	// Head lCache pointer
	l1Cache *lCache
	// Tail lCache pointer
	lnCache *lCache
	// Current number of caches
	numCaches uint8
	// Read pattern to be used while Get
	readPattern ReadPattern
	// Write pattern to be used while Put
	writePattern WritePattern
	// Cache value size cutoff
	maxValSize int
}

// NewMultiLevelCache creates a new mlcache object.
// It expects 0 < num(caches) <= maxCaches and pre-defined read/write patterns to be passed in.
func NewMultiLevelCache(readPattern ReadPattern, writePattern WritePattern, maxValSize int, caches ...Cacher) (Cacher, error) {

	numCaches := uint8(len(caches))

	if err := validate(numCaches, readPattern, writePattern); err != nil {
		return nil, err
	}

	ci := uint8(0)
	eCache := &lCache{cur: caches[ci], prev: nil, next: nil}
	sCache := eCache // save head
	ci++
	for ci < numCaches {
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

// Get executes a cache fetch given a key using pre-selected read pattern
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

// Get executes a cache add/update given a key, val and expiry time using pre-selected write pattern.
// It expects an absolute value of time (and not duration).
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
			}(200 * time.Millisecond) // write to higher level caches after waiting for this duration
		}
		return CacheStatusSuccess, nil
	}

	return CacheStatusSuccess, nil
}

// Del removes a cache item from all caches
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

// IsPresent checks if a particular key exists or not.
// It checks only in L1 cache assuming consistency between all cache levels.
// All inconsistencies must be handled using Get/Put methods.
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

// Flush clears all items from all cache levels
func (ca *cacher) Flush() error {

	this := ca.l1Cache
	for this != nil {
		cache := this.cur
		go cache.Flush() // async flush
		this = this.next
	}
	return nil
}
