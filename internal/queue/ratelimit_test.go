package queue

import (
	"context"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisRateLimiter_AcquireRelease(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	limiter := NewRedisRateLimiter(mr.Addr(), 3)

	ctx := context.Background()

	// Acquire should succeed.
	if err := limiter.Acquire(ctx, "tenant-1"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Counter should be 1.
	count, _ := mr.Get("tenant:tenant-1:active_jobs")
	if count != "1" {
		t.Fatalf("expected counter '1', got %q", count)
	}

	// Release should decrement.
	limiter.Release(ctx, "tenant-1")

	count, _ = mr.Get("tenant:tenant-1:active_jobs")
	if count != "0" {
		t.Fatalf("expected counter '0', got %q", count)
	}
}

func TestRedisRateLimiter_ExceedsLimit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	limiter := NewRedisRateLimiter(mr.Addr(), 2)

	ctx := context.Background()

	// Acquire twice — both should succeed (limit is 2).
	if err := limiter.Acquire(ctx, "tenant-1"); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if err := limiter.Acquire(ctx, "tenant-1"); err != nil {
		t.Fatalf("second Acquire: %v", err)
	}

	// Third acquire should fail — limit exceeded.
	err = limiter.Acquire(ctx, "tenant-1")
	if err == nil {
		t.Fatal("expected rate limit error, got nil")
	}

	// Release one and acquire again should succeed.
	limiter.Release(ctx, "tenant-1")
	if err := limiter.Acquire(ctx, "tenant-1"); err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
}

func TestRedisRateLimiter_ConcurrentSafety(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	limiter := NewRedisRateLimiter(mr.Addr(), 100)
	ctx := context.Background()

	var wg sync.WaitGroup
	const goroutines = 50

	// Each goroutine acquires then releases.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := limiter.Acquire(ctx, "tenant-1"); err != nil {
				return // limit exceeded is fine under contention
			}
			limiter.Release(ctx, "tenant-1")
		}()
	}

	wg.Wait()

	// Final counter should be 0.
	count, _ := mr.Get("tenant:tenant-1:active_jobs")
	if count != "0" {
		t.Fatalf("expected counter '0' after all goroutines, got %q", count)
	}
}

func TestRedisRateLimiter_DifferentTenants(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	limiter := NewRedisRateLimiter(mr.Addr(), 1)
	ctx := context.Background()

	// Each tenant should have independent limits.
	if err := limiter.Acquire(ctx, "tenant-a"); err != nil {
		t.Fatalf("Acquire tenant-a: %v", err)
	}
	if err := limiter.Acquire(ctx, "tenant-b"); err != nil {
		t.Fatalf("Acquire tenant-b: %v", err)
	}

	// Both should be at limit now.
	if err := limiter.Acquire(ctx, "tenant-a"); err == nil {
		t.Fatal("expected rate limit for tenant-a")
	}
	if err := limiter.Acquire(ctx, "tenant-b"); err == nil {
		t.Fatal("expected rate limit for tenant-b")
	}
}
