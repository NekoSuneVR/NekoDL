// Package scheduler manages the lifecycle of tasks across every download
// engine: it enforces a global concurrency limit, orders pending tasks by
// priority, and persists task metadata so the list survives a restart.
//
// It operates purely on the task.Task interface — it has no knowledge of
// HTTP, BitTorrent, yt-dlp, or any other engine. Engines construct their own
// task.Task implementations and hand them to Enqueue.
package scheduler

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

var ErrTaskNotFound = errors.New("scheduler: task not found")

// Options configure how a task is scheduled relative to others.
type Options struct {
	// Priority ranks pending tasks against each other; higher runs first.
	Priority int
	// MaxBandwidthBps caps this task's transfer rate. 0 means unlimited.
	// Enforcement is up to the engine that owns the task — the scheduler
	// only stores and reports this value until per-engine rate limiting exists.
	MaxBandwidthBps int64
}

// Record is a serializable snapshot of one scheduled task.
type Record struct {
	ID       string        `json:"id"`
	Engine   string        `json:"engine"`
	Priority int           `json:"priority"`
	AddedAt  time.Time     `json:"added_at"`
	Status   task.Status   `json:"status"`
	Progress task.Progress `json:"progress"`
	Error    string        `json:"error,omitempty"`
	Warning  string        `json:"warning,omitempty"`
}

type entry struct {
	task    task.Task
	opts    Options
	addedAt time.Time
}

// errorProvider is an optional capability: engines whose tasks can fail with
// a specific reason (e.g. httpengine.Task) implement it so that reason
// surfaces in Record.Error instead of a bare "status: error".
type errorProvider interface {
	Err() error
}

// warningProvider is an optional capability for a non-fatal caution about a
// task — currently only torrentengine.Task's "no proxy configured, your
// real IP is exposed" notice.
type warningProvider interface {
	Warning() string
}

func (e *entry) record() Record {
	rec := Record{
		ID:       e.task.ID(),
		Engine:   e.task.Engine(),
		Priority: e.opts.Priority,
		AddedAt:  e.addedAt,
		Status:   e.task.Status(),
		Progress: e.task.Progress(),
	}
	if ep, ok := e.task.(errorProvider); ok {
		if err := ep.Err(); err != nil {
			rec.Error = err.Error()
		}
	}
	if wp, ok := e.task.(warningProvider); ok {
		rec.Warning = wp.Warning()
	}
	return rec
}

// Scheduler manages a set of in-memory tasks. It is safe for concurrent use.
type Scheduler struct {
	mu            sync.Mutex
	maxConcurrent int
	entries       map[string]*entry
	store         *Store
}

// New creates a Scheduler. maxConcurrent <= 0 means unlimited concurrent
// tasks. store may be nil to disable persistence (e.g. in tests).
func New(maxConcurrent int, store *Store) *Scheduler {
	return &Scheduler{
		maxConcurrent: maxConcurrent,
		entries:       make(map[string]*entry),
		store:         store,
	}
}

// Enqueue registers a task with the scheduler and immediately re-evaluates
// which tasks should be active given the concurrency limit.
func (s *Scheduler) Enqueue(t task.Task, opts Options) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[t.ID()] = &entry{task: t, opts: opts, addedAt: time.Now()}
	s.rescheduleLocked()
	s.persistLocked()
}

// Records returns a snapshot of every known task, oldest first.
func (s *Scheduler) Records() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()

	records := make([]Record, 0, len(s.entries))
	for _, e := range s.entries {
		records = append(records, e.record())
	}
	sort.Slice(records, func(i, j int) bool {
		if !records[i].AddedAt.Equal(records[j].AddedAt) {
			return records[i].AddedAt.Before(records[j].AddedAt)
		}
		return records[i].ID < records[j].ID // deterministic tiebreak for same-instant inserts
	})
	return records
}

// Get returns a snapshot of one task by ID.
func (s *Scheduler) Get(id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return Record{}, ErrTaskNotFound
	}
	return e.record(), nil
}

// Pause pauses a task and re-evaluates scheduling (freeing a concurrency slot
// for the next-highest-priority pending task).
func (s *Scheduler) Pause(id string) error {
	return s.mutate(id, func(t task.Task) error { return t.Pause() })
}

// Resume marks a task eligible to run. Whether it actually starts running
// immediately depends on the concurrency limit — rescheduleLocked decides that.
func (s *Scheduler) Resume(id string) error {
	return s.mutate(id, func(t task.Task) error { return t.Resume() })
}

// Cancel stops a task without removing it from the scheduler's records.
func (s *Scheduler) Cancel(id string) error {
	return s.mutate(id, func(t task.Task) error { return t.Cancel() })
}

// Remove cancels a task (via its own Remove) and drops it from the scheduler entirely.
func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return ErrTaskNotFound
	}
	if err := e.task.Remove(); err != nil {
		return err
	}
	delete(s.entries, id)
	s.rescheduleLocked()
	s.persistLocked()
	return nil
}

func (s *Scheduler) mutate(id string, fn func(task.Task) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return ErrTaskNotFound
	}
	if err := fn(e.task); err != nil {
		return err
	}
	s.rescheduleLocked()
	s.persistLocked()
	return nil
}

// rescheduleLocked enforces maxConcurrent: the highest-priority tasks that
// aren't finished get resumed, and the rest get paused. Tasks that are
// complete, cancelled, or errored are left alone. Callers must hold s.mu.
func (s *Scheduler) rescheduleLocked() {
	if s.maxConcurrent <= 0 {
		return
	}

	runnable := make([]*entry, 0, len(s.entries))
	for _, e := range s.entries {
		switch e.task.Status() {
		case task.StatusComplete, task.StatusCancelled, task.StatusError:
			continue
		}
		runnable = append(runnable, e)
	}

	sort.Slice(runnable, func(i, j int) bool {
		if runnable[i].opts.Priority != runnable[j].opts.Priority {
			return runnable[i].opts.Priority > runnable[j].opts.Priority
		}
		if !runnable[i].addedAt.Equal(runnable[j].addedAt) {
			return runnable[i].addedAt.Before(runnable[j].addedAt)
		}
		// Deterministic tiebreak: without this, two tasks enqueued close enough
		// together to get the same timestamp would have their relative order
		// depend on Go's randomized map iteration order feeding into this sort,
		// making "which task runs" flip randomly between otherwise-identical runs.
		return runnable[i].task.ID() < runnable[j].task.ID()
	})

	for i, e := range runnable {
		if i < s.maxConcurrent {
			if e.task.Status() != task.StatusActive {
				_ = e.task.Resume()
			}
		} else if e.task.Status() != task.StatusPaused {
			_ = e.task.Pause()
		}
	}
}

// PersistPeriodically saves a snapshot every interval until ctx is done.
// This matters because tasks change status on their own in the background
// (a download completing, erroring, etc.) without calling back into the
// Scheduler — without this, the on-disk snapshot only updates on the next
// explicit Enqueue/Pause/Resume/Cancel/Remove call, which may never come
// for a task that just runs to completion by itself.
func (s *Scheduler) PersistPeriodically(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.persistLocked()
			s.mu.Unlock()
		}
	}
}

func (s *Scheduler) persistLocked() {
	if s.store == nil {
		return
	}
	records := make([]Record, 0, len(s.entries))
	for _, e := range s.entries {
		records = append(records, e.record())
	}
	_ = s.store.Save(records)
}
