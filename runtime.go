package gostream

import (
	"sync"
	"sync/atomic"

	"go.uber.org/multierr"
	"go.viam.com/utils"
)

// runParallel runs the given functions in parallel to completion or error.
func runParallel(fs []func() error) error {
	var wg sync.WaitGroup
	wg.Add(len(fs))
	errs := make([]error, len(fs))
	var numErrs int32
	for i, f := range fs {
		iCopy := i
		fCopy := f
		utils.PanicCapturingGo(func() {
			defer wg.Done()
			err := fCopy()
			if err != nil {
				errs[iCopy] = err
				atomic.AddInt32(&numErrs, 1)
			}
		})
	}
	wg.Wait()

	return multierr.Combine(errs...)
}
