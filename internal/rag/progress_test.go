package rag

import (
	"sync"
	"testing"
)

func TestProgressInitial(t *testing.T) {
	p := NewProgress()
	status := p.Status()

	if status.State != "idle" {
		t.Errorf("expected state idle, got %q", status.State)
	}
	if status.TotalFiles != 0 {
		t.Errorf("expected 0 total files, got %d", status.TotalFiles)
	}
	if status.Percent != 0 {
		t.Errorf("expected 0 percent, got %d", status.Percent)
	}
}

func TestProgressBegin(t *testing.T) {
	p := NewProgress()
	p.Begin("indexing", 10)

	status := p.Status()
	if status.State != "indexing" {
		t.Errorf("expected state indexing, got %q", status.State)
	}
	if status.TotalFiles != 10 {
		t.Errorf("expected 10 total files, got %d", status.TotalFiles)
	}
}

func TestProgressIncrements(t *testing.T) {
	p := NewProgress()
	p.Begin("indexing", 4)

	p.RecordIndexed()
	p.RecordIndexed()
	p.RecordFailed()

	status := p.Status()
	if status.IndexedFiles != 2 {
		t.Errorf("expected 2 indexed, got %d", status.IndexedFiles)
	}
	if status.FailedFiles != 1 {
		t.Errorf("expected 1 failed, got %d", status.FailedFiles)
	}
	if status.Percent != 75 {
		t.Errorf("expected 75 percent, got %d", status.Percent)
	}
	if status.State != "indexing" {
		t.Errorf("expected state indexing, got %q", status.State)
	}
}

func TestProgressComplete(t *testing.T) {
	p := NewProgress()
	p.Begin("indexing", 2)
	p.RecordIndexed()
	p.RecordIndexed()

	status := p.Status()
	if status.Percent != 100 {
		t.Errorf("expected 100 percent, got %d", status.Percent)
	}
	if status.State != "idle" {
		t.Errorf("expected state idle after completion, got %q", status.State)
	}
}

func TestProgressIgnoresUpdatesOutsideActiveBatch(t *testing.T) {
	p := NewProgress()
	p.Begin("indexing", 1)
	p.RecordIndexed()
	p.RecordIndexed()
	p.RecordFailed()

	status := p.Status()
	if status.IndexedFiles != 1 {
		t.Errorf("expected indexed files to remain 1, got %d", status.IndexedFiles)
	}
	if status.FailedFiles != 0 {
		t.Errorf("expected failed files to remain 0, got %d", status.FailedFiles)
	}
	if status.Percent != 100 {
		t.Errorf("expected 100 percent, got %d", status.Percent)
	}
}

func TestProgressConcurrency(t *testing.T) {
	p := NewProgress()
	p.Begin("indexing", 100)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.RecordIndexed()
		}()
	}
	wg.Wait()

	status := p.Status()
	if status.IndexedFiles != 100 {
		t.Errorf("expected 100 indexed, got %d", status.IndexedFiles)
	}
	if status.Percent != 100 {
		t.Errorf("expected 100 percent, got %d", status.Percent)
	}
}

func TestProgressSetState(t *testing.T) {
	p := NewProgress()
	p.SetState("rebuilding")

	status := p.Status()
	if status.State != "rebuilding" {
		t.Errorf("expected state rebuilding, got %q", status.State)
	}
}

func TestProgressFinishZeroBatch(t *testing.T) {
	p := NewProgress()
	p.Begin("rebuilding", 0)

	status := p.Status()
	if status.State != "idle" {
		t.Errorf("expected idle for empty batch, got %q", status.State)
	}
	if status.Percent != 0 {
		t.Errorf("expected 0 percent, got %d", status.Percent)
	}
}
