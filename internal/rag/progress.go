package rag

import "sync"

// Progress tracks indexing progress in a thread-safe manner.
type Progress struct {
	mu           sync.RWMutex
	state        string
	totalFiles   int
	indexedFiles int
	failedFiles  int
	active       bool
}

// NewProgress creates a new progress tracker.
func NewProgress() *Progress {
	return &Progress{state: "idle"}
}

// Begin starts a new tracked batch.
func (p *Progress) Begin(state string, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state == "" {
		state = "indexing"
	}

	p.totalFiles = total
	p.indexedFiles = 0
	p.failedFiles = 0
	p.state = state
	p.active = total > 0
	if total == 0 {
		p.state = "idle"
	}
}

// RecordIndexed increments the indexed file counter for the active batch.
func (p *Progress) RecordIndexed() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		return
	}

	p.indexedFiles++
	p.finishIfDoneLocked()
}

// RecordFailed increments the failed file counter for the active batch.
func (p *Progress) RecordFailed() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		return
	}

	p.failedFiles++
	p.finishIfDoneLocked()
}

// SetState sets the state directly.
func (p *Progress) SetState(state string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = state
}

// Finish marks the active batch as complete.
func (p *Progress) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active = false
	p.state = "idle"
}

// Status returns a snapshot of the current indexing status.
func (p *Progress) Status() IndexStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pct := 0
	if p.totalFiles > 0 {
		pct = (p.indexedFiles + p.failedFiles) * 100 / p.totalFiles
	}

	return IndexStatus{
		State:        p.state,
		TotalFiles:   p.totalFiles,
		IndexedFiles: p.indexedFiles,
		FailedFiles:  p.failedFiles,
		Percent:      pct,
	}
}

func (p *Progress) finishIfDoneLocked() {
	if p.indexedFiles+p.failedFiles < p.totalFiles || p.totalFiles <= 0 {
		return
	}

	p.active = false
	p.state = "idle"
}
