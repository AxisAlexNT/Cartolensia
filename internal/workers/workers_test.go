package workers

import (
	"context"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
)

func TestWorkerRecordsPanicAsFailedJob(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	job := jobs.New("panic", nil)
	job.MaxAttempts = 1
	if _, err := store.EnqueueJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	manager := New(store, Config{
		WorkerID:          "worker-test",
		PollInterval:      10 * time.Millisecond,
		LeaseDuration:     200 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		MaxConcurrency:    1,
	})
	manager.Register("panic", func(context.Context, *jobs.Job) error {
		panic("boom")
	})
	manager.Start()
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Stop(stopCtx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == jobs.StatusFailed {
			if current.Error == "" {
				t.Fatalf("failed job has no error: %#v", current)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	current, _ := store.GetJob(ctx, job.ID)
	t.Fatalf("job did not fail after panic: %#v", current)
}

func TestWorkerReleasesExpiredLeasesOnPoll(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()

	stale := jobs.New("stale", nil)
	stale.Status = jobs.StatusRunning
	stale.WorkerID = "old-worker"
	expired := time.Now().Add(-time.Minute)
	stale.LeaseExpiresAt = &expired
	if _, err := store.EnqueueJob(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateJob(ctx, stale); err != nil {
		t.Fatal(err)
	}

	manager := New(store, Config{
		WorkerID:          "worker-test",
		PollInterval:      10 * time.Millisecond,
		LeaseDuration:     200 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		MaxConcurrency:    1,
	})
	manager.Register("other", func(context.Context, *jobs.Job) error { return nil })
	manager.Start()
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Stop(stopCtx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.GetJob(ctx, stale.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == jobs.StatusQueued && current.WorkerID == "" && current.LeaseExpiresAt == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	current, _ := store.GetJob(ctx, stale.ID)
	t.Fatalf("expired lease was not released: %#v", current)
}
