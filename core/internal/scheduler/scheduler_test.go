package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/task"
)

// fakeTask is a minimal task.Task used only by these tests — no real engine
// exists yet to exercise the scheduler against.
type fakeTask struct {
	id     string
	status task.Status
}

func newFakeTask(id string) *fakeTask {
	return &fakeTask{id: id, status: task.StatusPending}
}

func (f *fakeTask) ID() string               { return f.id }
func (f *fakeTask) Engine() string           { return "fake" }
func (f *fakeTask) Pause() error             { f.status = task.StatusPaused; return nil }
func (f *fakeTask) Resume() error            { f.status = task.StatusActive; return nil }
func (f *fakeTask) Cancel() error            { f.status = task.StatusCancelled; return nil }
func (f *fakeTask) Remove() error            { return nil }
func (f *fakeTask) Progress() task.Progress  { return task.Progress{} }
func (f *fakeTask) Status() task.Status      { return f.status }

func TestEnqueueAndRecords(t *testing.T) {
	s := New(0, nil)
	s.Enqueue(newFakeTask("a"), Options{})
	s.Enqueue(newFakeTask("b"), Options{})

	records := s.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].ID != "a" || records[1].ID != "b" {
		t.Fatalf("expected records ordered by insertion time, got %+v", records)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	s := New(1, nil)
	s.Enqueue(newFakeTask("a"), Options{})
	s.Enqueue(newFakeTask("b"), Options{})

	recA, err := s.Get("a")
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	recB, err := s.Get("b")
	if err != nil {
		t.Fatalf("get b: %v", err)
	}

	if recA.Status != task.StatusActive {
		t.Fatalf("expected task a to be active, got %s", recA.Status)
	}
	if recB.Status != task.StatusPaused {
		t.Fatalf("expected task b to be paused under a 1-task concurrency limit, got %s", recB.Status)
	}
}

func TestPriorityOrdering(t *testing.T) {
	s := New(1, nil)
	s.Enqueue(newFakeTask("low"), Options{Priority: 0})
	s.Enqueue(newFakeTask("high"), Options{Priority: 10})

	recHigh, _ := s.Get("high")
	recLow, _ := s.Get("low")

	if recHigh.Status != task.StatusActive {
		t.Fatalf("expected higher-priority task to be active, got %s", recHigh.Status)
	}
	if recLow.Status != task.StatusPaused {
		t.Fatalf("expected lower-priority task to be paused, got %s", recLow.Status)
	}
}

func TestMissingTaskErrors(t *testing.T) {
	s := New(0, nil)

	if _, err := s.Get("missing"); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
	if err := s.Pause("missing"); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
	if err := s.Remove("missing"); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestRemove(t *testing.T) {
	s := New(0, nil)
	s.Enqueue(newFakeTask("a"), Options{})

	if err := s.Remove("a"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := s.Get("a"); err != ErrTaskNotFound {
		t.Fatalf("expected task to be gone after Remove, got %v", err)
	}
}

func TestPersistPeriodicallyCapturesOutOfBandStatusChanges(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	s := New(0, store)

	ft := newFakeTask("a")
	s.Enqueue(ft, Options{})

	// Simulate the task completing entirely on its own — exactly what a real
	// engine's background goroutine does — without going through any
	// Scheduler method that would normally trigger a save.
	ft.status = task.StatusComplete

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.PersistPeriodically(ctx, 20*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if records, err := store.Load(); err == nil && len(records) == 1 && records[0].Status == task.StatusComplete {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected the periodic persister to eventually pick up the task's out-of-band status change")
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	want := []Record{{ID: "a", Engine: "fake", Status: task.StatusActive}}
	if err := store.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("expected round-tripped record with ID 'a', got %+v", got)
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	got, err := store.Load()
	if err != nil {
		t.Fatalf("expected no error for a missing snapshot, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil records for a missing snapshot, got %+v", got)
	}
}
