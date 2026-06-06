package jobs

import (
	"errors"
	"testing"
)

func TestJobStateTransitions(t *testing.T) {
	job := New("discovery", nil)
	if err := Start(&job); err != nil {
		t.Fatal(err)
	}
	if job.Status != StatusRunning || job.StartedAt == nil {
		t.Fatalf("job did not start: %#v", job)
	}
	if err := Complete(&job); err != nil {
		t.Fatal(err)
	}
	if job.Status != StatusSucceeded || job.FinishedAt == nil {
		t.Fatalf("job did not complete: %#v", job)
	}
}

func TestJobRejectsInvalidTransition(t *testing.T) {
	job := New("discovery", nil)
	if err := Complete(&job); err == nil {
		t.Fatal("expected invalid transition")
	}
}

func TestJobFailRecordsError(t *testing.T) {
	job := New("hash", nil)
	if err := Start(&job); err != nil {
		t.Fatal(err)
	}
	if err := Fail(&job, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if job.Status != StatusFailed || job.Error != "boom" {
		t.Fatalf("unexpected failure state %#v", job)
	}
}

func TestJobCancellationStates(t *testing.T) {
	queued := New("discovery", nil)
	if err := RequestCancel(&queued); err != nil {
		t.Fatal(err)
	}
	if queued.Status != StatusCanceled || queued.FinishedAt == nil || queued.CancelRequestedAt == nil {
		t.Fatalf("queued job not canceled: %#v", queued)
	}

	running := New("hash", nil)
	if err := Start(&running); err != nil {
		t.Fatal(err)
	}
	if err := RequestCancel(&running); err != nil {
		t.Fatal(err)
	}
	if running.Status != StatusCancelRequested || running.CancelRequestedAt == nil {
		t.Fatalf("running job not marked for cancellation: %#v", running)
	}
	if err := Cancel(&running); err != nil {
		t.Fatal(err)
	}
	if running.Status != StatusCanceled || running.FinishedAt == nil {
		t.Fatalf("running job not canceled: %#v", running)
	}
}

func TestStartDoesNotDoubleCountLeasedJob(t *testing.T) {
	job := New("discovery", nil)
	if err := Start(&job); err != nil {
		t.Fatal(err)
	}
	attempts := job.Attempts
	if err := Start(&job); err != nil {
		t.Fatal(err)
	}
	if job.Attempts != attempts {
		t.Fatalf("attempts changed for already running job: %#v", job)
	}
}
