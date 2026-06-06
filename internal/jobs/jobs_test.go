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
