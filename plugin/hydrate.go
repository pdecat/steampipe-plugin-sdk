package plugin

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/turbot/go-kit/helpers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryHydrate function invokes the hydrate function with retryable errors and retries the function until the maximum attemptes before throwing error
func RetryHydrate(ctx context.Context, d *QueryData, hydrateData *HydrateData, hydrateFunc HydrateFunc, retryConfig *RetryConfig) (interface{}, error) {
	backoff, err := retry.NewFibonacci(100 * time.Millisecond)
	if err != nil {
		return nil, err
	}
	var hydrateResult interface{}
	shouldRetryErrorFunc := retryConfig.ShouldRetryError

	err = retry.Do(ctx, retry.WithMaxRetries(10, backoff), func(ctx context.Context) error {
		hydrateResult, err = hydrateFunc(ctx, d, hydrateData)
		if err != nil && shouldRetryErrorFunc(err) {
			err = retry.RetryableError(err)
		}
		return err
	})
	return hydrateResult, err
}

// WrapHydrate is a higher order function which returns a HydrateFunc which handles Ignorable errors
func WrapHydrate(hydrateFunc HydrateFunc, ignoreConfig *IgnoreConfig) HydrateFunc {
	log.Printf("[TRACE] WrapHydrate %s, ignore config %s\n", helpers.GetFunctionName(hydrateFunc), ignoreConfig.String())

	return func(ctx context.Context, d *QueryData, h *HydrateData) (item interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[WARN] recovered a panic from a wrapped hydrate function: %v\n", r)
				err = status.Error(codes.Internal, fmt.Sprintf("hydrate function %s failed with panic %v", helpers.GetFunctionName(hydrateFunc), r))
			}
		}()
		// call the underlying get function
		item, err = hydrateFunc(ctx, d, h)
		if err != nil {
			log.Printf("[TRACE] wrapped hydrate call %s returned error %v, ignore config %s\n", helpers.GetFunctionName(hydrateFunc), err, ignoreConfig.String())
			// see if the ignoreConfig defines a should ignore function
			if ignoreConfig.ShouldIgnoreError != nil && ignoreConfig.ShouldIgnoreError(err) {
				log.Printf("[TRACE] wrapped hydrate call %s returned error but we are ignoring it: %v", helpers.GetFunctionName(hydrateFunc), err)
				return nil, nil
			}
			if ignoreConfig.ShouldIgnoreErrorFunc != nil && ignoreConfig.ShouldIgnoreErrorFunc(ctx, d, h, err) {
				log.Printf("[TRACE] wrapped hydrate call %s returned error but we are ignoring it: %v", helpers.GetFunctionName(hydrateFunc), err)
				return nil, nil
			}
			// pass any other error on
			return nil, err
		}
		return item, nil
	}
}
