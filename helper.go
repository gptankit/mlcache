package mlcache

import "github.com/gptankit/mlcache/errs"

// validate checks if the input parameters are within allowed limits
func validate(numCaches uint8, readPattern ReadPattern, writePattern WritePattern) error {

	if numCaches == 0 { // if called with no cache parameters
		return errs.New(NoWorkableCacheFound)
	} else if numCaches > maxCaches { // if called with more than maxCache limit
		return errs.New(MaxCacheLevelExceeded)
	}

	// if invalid readPattern selected
	if readPattern >= endR {
		return errs.New(InvalidReadPattern)
	}

	// if invalid writePattern selected
	if writePattern >= endW {
		return errs.New(InvalidWritePattern)
	}

	return nil
}
