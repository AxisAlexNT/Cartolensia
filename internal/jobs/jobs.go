package jobs

import (
	"errors"
	"fmt"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type Job struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	Status          Status     `json:"status"`
	Payload         any        `json:"payload,omitempty"`
	ProgressCurrent int64      `json:"progress_current"`
	ProgressTotal   *int64     `json:"progress_total,omitempty"`
	Counters        Counters   `json:"counters"`
	Attempts        int        `json:"attempts"`
	MaxAttempts     int        `json:"max_attempts"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	Error           string     `json:"error,omitempty"`
	Logs            []LogLine  `json:"logs,omitempty"`
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
		StatusQueued:  {StatusRunning, StatusCanceled},
		StatusRunning: {StatusSucceeded, StatusFailed, StatusCanceled},
		StatusFailed:  {StatusQueued},
	}
	for _, status := range allowed[current] {
		if status == next {
			return nil
		}
	}
	return fmt.Errorf("invalid job transition %s -> %s", current, next)
}

func Start(job *Job) error {
	if err := Transition(job.Status, StatusRunning); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Status = StatusRunning
	job.StartedAt = &now
	job.Attempts++
	return nil
}

func Complete(job *Job) error {
	if err := Transition(job.Status, StatusSucceeded); err != nil {
		return err
	}
	now := time.Now().UTC()
	job.Status = StatusSucceeded
	job.FinishedAt = &now
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
	return nil
}

func AddLog(job *Job, level, message string) {
	job.Logs = append(job.Logs, LogLine{Level: level, Message: message, CreatedAt: time.Now().UTC()})
	if len(job.Logs) > 200 {
		job.Logs = job.Logs[len(job.Logs)-200:]
	}
}
