package jobs

import (
	"errors"
	"fmt"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

type Status string

const (
	StatusQueued          Status = "queued"
	StatusRunning         Status = "running"
	StatusCancelRequested Status = "cancel_requested"
	StatusSucceeded       Status = "succeeded"
	StatusFailed          Status = "failed"
	StatusCanceled        Status = "canceled"
)

var ErrCanceled = errors.New("job canceled")

type Job struct {
	ID                string     `json:"id"`
	Kind              string     `json:"kind"`
	Status            Status     `json:"status"`
	Payload           any        `json:"payload,omitempty"`
	ProgressCurrent   int64      `json:"progress_current"`
	ProgressTotal     *int64     `json:"progress_total,omitempty"`
	Counters          Counters   `json:"counters"`
	Attempts          int        `json:"attempts"`
	MaxAttempts       int        `json:"max_attempts"`
	WorkerID          string     `json:"worker_id,omitempty"`
	LeaseExpiresAt    *time.Time `json:"lease_expires_at,omitempty"`
	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`
	NextRunAt         *time.Time `json:"next_run_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	Error             string     `json:"error,omitempty"`
	Logs              []LogLine  `json:"logs,omitempty"`
}

type Counters struct {
	Scanned int64 `json:"scanned"`
	Created int64 `json:"created"`
	Updated int64 `json:"updated"`
	Hashed  int64 `json:"hashed"`
	Bytes   int64 `json:"bytes"`
	Errors  int64 `json:"errors"`
}

type LogLine struct {
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

func New(kind string, payload any) Job {
	now := time.Now().UTC()
	return Job{
		ID:          id.NewUUID(),
		Kind:        kind,
		Status:      StatusQueued,
		Payload:     payload,
		MaxAttempts: 3,
		CreatedAt:   now,
	}
}

func Transition(current Status, next Status) error {
	if current == next {
		return nil
	}
	allowed := map[Status][]Status{
		StatusQueued:          {StatusRunning, StatusCancelRequested, StatusCanceled},
		StatusRunning:         {StatusSucceeded, StatusFailed, StatusCancelRequested, StatusCanceled, StatusQueued},
		StatusCancelRequested: {StatusCanceled, StatusFailed, StatusQueued},
		StatusFailed:          {StatusQueued},
	}
	for _, status := range allowed[current] {
		if status == next {
			return nil
		}
	}
	return fmt.Errorf("invalid job transition %s -> %s", current, next)
}

func Start(job *Job) error {
	if job.Status == StatusRunning {
		if job.StartedAt == nil {
			now := time.Now().UTC()
			job.StartedAt = &now
		}
		return nil
	}
	if err := Transition(job.Status, StatusRunning); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Status = StatusRunning
	job.StartedAt = &now
	job.Attempts++
	job.Error = ""
	return nil
}

func Complete(job *Job) error {
	if err := Transition(job.Status, StatusSucceeded); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Status = StatusSucceeded
	job.FinishedAt = &now
	job.WorkerID = ""
	job.LeaseExpiresAt = nil
	return nil
}

func Fail(job *Job, cause error) error {
	if cause == nil {
		return errors.New("job failure cause is required")
	}
	if err := Transition(job.Status, StatusFailed); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Status = StatusFailed
	job.FinishedAt = &now
	job.Error = cause.Error()
	job.WorkerID = ""
	job.LeaseExpiresAt = nil
	return nil
}

func RequestCancel(job *Job) error {
	now := time.Now().UTC()
	switch job.Status {
	case StatusQueued:
		job.Status = StatusCanceled
		job.CancelRequestedAt = &now
		job.FinishedAt = &now
		return nil
	case StatusRunning:
		job.Status = StatusCancelRequested
		job.CancelRequestedAt = &now
		return nil
	case StatusCancelRequested, StatusCanceled, StatusSucceeded, StatusFailed:
		if job.CancelRequestedAt == nil && (job.Status == StatusCancelRequested || job.Status == StatusCanceled) {
			job.CancelRequestedAt = &now
		}
		return nil
	default:
		return fmt.Errorf("invalid job status %q", job.Status)
	}
}

func Cancel(job *Job) error {
	if job.Status == StatusCanceled {
		return nil
	}
	if err := Transition(job.Status, StatusCanceled); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Status = StatusCanceled
	if job.CancelRequestedAt == nil {
		job.CancelRequestedAt = &now
	}
	job.FinishedAt = &now
	job.WorkerID = ""
	job.LeaseExpiresAt = nil
	return nil
}

func Retry(job *Job, delay time.Duration, cause error) error {
	if cause == nil {
		return errors.New("job retry cause is required")
	}
	if err := Transition(job.Status, StatusQueued); err != nil {
		return err
	}
	next := time.Now().UTC().Add(delay)
	job.Status = StatusQueued
	job.NextRunAt = &next
	job.WorkerID = ""
	job.LeaseExpiresAt = nil
	job.Error = cause.Error()
	return nil
}

func AddLog(job *Job, level, message string) {
	job.Logs = append(job.Logs, LogLine{Level: level, Message: message, CreatedAt: time.Now().UTC()})
	if len(job.Logs) > 200 {
		job.Logs = job.Logs[len(job.Logs)-200:]
	}
}
