package workers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
)

type Config struct {
	WorkerID          string
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	MaxConcurrency    int
}

type Handler func(context.Context, *jobs.Job) error

type Manager struct {
	store    catalog.Store
	cfg      Config
	handlers map[string]Handler
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	slots    chan struct{}
}

func New(store catalog.Store, cfg Config) *Manager {
	if cfg.WorkerID == "" {
		cfg.WorkerID = fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 30 * time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = cfg.LeaseDuration / 3
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:    store,
		cfg:      cfg,
		handlers: map[string]Handler{},
		ctx:      ctx,
		cancel:   cancel,
		slots:    make(chan struct{}, cfg.MaxConcurrency),
	}
}

func (m *Manager) Register(kind string, handler Handler) {
	m.handlers[kind] = handler
}

func (m *Manager) Start() {
	m.wg.Add(1)
	go m.loop()
}

func (m *Manager) Stop(ctx context.Context) error {
	m.cancel()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) WorkerID() string {
	return m.cfg.WorkerID
}

func (m *Manager) LeaseDuration() time.Duration {
	return m.cfg.LeaseDuration
}

func (m *Manager) loop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		m.releaseExpiredLeases()
		m.dispatchAvailable()
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) releaseExpiredLeases() {
	released, err := m.store.ReleaseExpiredLeases(m.ctx, time.Now().UTC())
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("cartolensia worker lease cleanup failed: %v", err)
		return
	}
	if released > 0 {
		log.Printf("cartolensia worker returned %d expired job lease(s) to the queue", released)
	}
}

func (m *Manager) dispatchAvailable() {
	for len(m.slots) < cap(m.slots) {
		select {
		case m.slots <- struct{}{}:
		case <-m.ctx.Done():
			return
		}
		job, err := m.store.LeaseNextJob(m.ctx, m.cfg.WorkerID, m.kinds(), m.cfg.LeaseDuration)
		if err != nil {
			<-m.slots
			if !errors.Is(err, catalog.ErrNotFound) && !errors.Is(err, context.Canceled) {
				log.Printf("cartolensia worker lease failed: %v", err)
			}
			return
		}
		handler, ok := m.handlers[job.Kind]
		if !ok {
			<-m.slots
			_ = m.store.FailLeasedJob(m.ctx, job, m.cfg.WorkerID, fmt.Errorf("no handler registered for job kind %q", job.Kind))
			continue
		}
		m.wg.Add(1)
		go m.run(job, handler)
	}
}

func (m *Manager) run(job jobs.Job, handler Handler) {
	defer m.wg.Done()
	defer func() { <-m.slots }()
	done := make(chan struct{})
	defer close(done)
	go m.heartbeat(job.ID, done)
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = m.store.FailLeasedJob(m.ctx, job, m.cfg.WorkerID, jobs.PanicError(fmt.Errorf("panic: %v", recovered)))
		}
	}()
	err := handler(m.ctx, &job)
	if err == nil || errors.Is(err, jobs.ErrCanceled) || errors.Is(err, catalog.ErrJobLeaseLost) {
		return
	}
	if failErr := m.store.FailLeasedJob(m.ctx, job, m.cfg.WorkerID, err); failErr != nil && !errors.Is(failErr, catalog.ErrJobLeaseLost) {
		log.Printf("cartolensia worker failed to record job failure for %s: %v", job.ID, failErr)
	}
}

func (m *Manager) heartbeat(jobID string, done <-chan struct{}) {
	ticker := time.NewTicker(m.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if err := m.store.HeartbeatJob(m.ctx, jobID, m.cfg.WorkerID, m.cfg.LeaseDuration); err != nil {
				if !errors.Is(err, catalog.ErrJobLeaseLost) && !errors.Is(err, catalog.ErrNotFound) && !errors.Is(err, context.Canceled) {
					log.Printf("cartolensia worker heartbeat failed for %s: %v", jobID, err)
				}
				return
			}
		}
	}
}

func (m *Manager) kinds() []string {
	kinds := make([]string, 0, len(m.handlers))
	for kind := range m.handlers {
		kinds = append(kinds, kind)
	}
	return kinds
}
