package rag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
)

// FileIndexer is the interface that the watcher uses to index and remove files.
// Defined for testability.
type FileIndexer interface {
	CanIndex(path string) bool
	IndexFile(ctx context.Context, path string) error
	BatchIndexFile(ctx context.Context, path string) error
	RemoveFile(ctx context.Context, path string) error
}

// Watcher monitors filesystem paths and triggers indexing for changes.
type Watcher struct {
	indexer  FileIndexer
	cfg      *config.RAGConfig
	logger   *zap.Logger
	watcher  *fsnotify.Watcher
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	progress *Progress

	// debounce timers per path
	mu     sync.Mutex
	timers map[string]*time.Timer
}

// NewWatcher creates a file watcher that monitors configured paths.
func NewWatcher(indexer FileIndexer, cfg *config.RAGConfig, progress *Progress, logger *zap.Logger) *Watcher {
	return &Watcher{
		indexer:  indexer,
		cfg:      cfg,
		logger:   logger,
		progress: progress,
		timers:   make(map[string]*time.Timer),
	}
}

// Start begins watching in a background goroutine. It first performs an initial
// scan of all configured paths, then listens for fsnotify events.
func (w *Watcher) Start(ctx context.Context) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = fsw
	w.ctx, w.cancel = context.WithCancel(ctx)

	// Add all watch paths recursively.
	for _, p := range w.cfg.WatchPaths {
		expanded := expandHome(p)
		if err := w.addRecursive(expanded); err != nil {
			w.logger.Warn("failed to add watch path", zap.String("path", expanded), zap.Error(err))
		}
	}

	// Initial scan in background.
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.initialScan()
	}()

	// Event loop.
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.eventLoop()
	}()

	w.logger.Info("file watcher started", zap.Strings("paths", w.cfg.WatchPaths))
	return nil
}

// Stop halts the watcher and waits for goroutines to finish.
// It closes the fsnotify watcher first so the event loop unblocks immediately,
// then waits up to 5 seconds for the initial scan goroutine to notice
// cancellation. If it does not finish in time, Stop returns anyway.
func (w *Watcher) Stop() error {
	if w.cancel != nil {
		w.cancel()
	}
	// Close fsnotify first: this makes eventLoop exit via the closed-channel
	// path instead of waiting for ctx.Done() to be selected.
	var closeErr error
	if w.watcher != nil {
		closeErr = w.watcher.Close()
	}
	// Wait for goroutines with a hard timeout so a slow initialScan (e.g.
	// embedding a large file) cannot block shutdown indefinitely.
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		w.logger.Warn("watcher stop timed out; some in-progress indexing may have been abandoned")
	}
	return closeErr
}

func (w *Watcher) initialScan() {
	// Count qualifying files first.
	var paths []string
	for _, p := range w.cfg.WatchPaths {
		expanded := expandHome(p)
		_ = filepath.WalkDir(expanded, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if w.shouldIgnoreDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if w.isQualified(path) && w.indexer.CanIndex(path) {
				paths = append(paths, path)
			}
			return nil
		})
	}

	if w.progress != nil {
		w.progress.Begin("indexing", len(paths))
		defer w.progress.Finish()
	}

	for _, path := range paths {
		if w.ctx.Err() != nil {
			return
		}
		if err := w.indexer.BatchIndexFile(w.ctx, path); err != nil {
			w.logger.Debug("initial index failed", zap.String("path", path), zap.Error(err))
		}
	}
}

func (w *Watcher) eventLoop() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn("fsnotify error", zap.Error(err))
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// New directory — add watcher.
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			if !w.shouldIgnoreDir(filepath.Base(path)) {
				_ = w.addRecursive(path)
			}
			return
		}
	}

	// Remove event.
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		if w.isQualified(path) && w.indexer.CanIndex(path) {
			w.debounce(path, func() {
				if err := w.indexer.RemoveFile(w.ctx, path); err != nil {
					w.logger.Debug("remove failed", zap.String("path", path), zap.Error(err))
				}
			})
		}
		return
	}

	// Create or write event — debounce and index.
	if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
		if w.isQualified(path) && w.indexer.CanIndex(path) {
			w.debounce(path, func() {
				if err := w.indexer.IndexFile(w.ctx, path); err != nil {
					w.logger.Debug("index failed", zap.String("path", path), zap.Error(err))
				}
			})
		}
	}
}

func (w *Watcher) debounce(path string, fn func()) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if t, ok := w.timers[path]; ok {
		t.Stop()
	}
	w.timers[path] = time.AfterFunc(100*time.Millisecond, fn)
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if w.shouldIgnoreDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			if err := w.watcher.Add(path); err != nil {
				w.logger.Debug("failed to watch dir", zap.String("path", path), zap.Error(err))
			}
		}
		return nil
	})
}

func (w *Watcher) shouldIgnoreDir(name string) bool {
	for _, d := range w.cfg.IgnoreDirs {
		if d == name {
			return true
		}
	}
	return false
}

func (w *Watcher) isQualified(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, e := range w.cfg.Extensions {
		if strings.EqualFold(e, ext) {
			return true
		}
	}
	return false
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
