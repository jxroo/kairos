package rag

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/memory"
)

// Indexer orchestrates the document indexing pipeline:
// detect → stat → hash → parse → chunk → embed → store.
type Indexer struct {
	store    *Store
	embedder memory.Embedder
	index    memory.VectorIndex
	bleve    *BleveWrapper
	registry *ParserRegistry
	chunker  *Chunker
	progress *Progress
	cfg      *config.RAGConfig
	logger   *zap.Logger
}

// NewIndexer creates an Indexer with the given dependencies.
func NewIndexer(
	store *Store,
	embedder memory.Embedder,
	index memory.VectorIndex,
	bleve *BleveWrapper,
	registry *ParserRegistry,
	chunker *Chunker,
	progress *Progress,
	cfg *config.RAGConfig,
	logger *zap.Logger,
) *Indexer {
	return &Indexer{
		store:    store,
		embedder: embedder,
		index:    index,
		bleve:    bleve,
		registry: registry,
		chunker:  chunker,
		progress: progress,
		cfg:      cfg,
		logger:   logger,
	}
}

// CanIndex reports whether the file extension is both configured and supported.
func (idx *Indexer) CanIndex(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	return idx.registry.Supported(ext) && idx.isAllowedExtension(ext)
}

// IndexFile indexes a single file: stat → hash → parse → chunk → embed → store.
// If the file has already been indexed with the same hash, it is skipped.
func (idx *Indexer) IndexFile(ctx context.Context, path string) error {
	return idx.indexFile(ctx, path, false)
}

// BatchIndexFile indexes a file as part of a tracked batch operation.
func (idx *Indexer) BatchIndexFile(ctx context.Context, path string) error {
	return idx.indexFile(ctx, path, true)
}

func (idx *Indexer) indexFile(ctx context.Context, path string, trackProgress bool) error {
	// Stat the file.
	info, err := os.Stat(path)
	if err != nil {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("stat %q: %w", path, err)
	}
	if info.IsDir() {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("path %q is a directory", path)
	}

	// Check max file size.
	if idx.cfg != nil && idx.cfg.MaxFileSize > 0 && info.Size() > idx.cfg.MaxFileSize {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("file %q exceeds max size (%d > %d)", path, info.Size(), idx.cfg.MaxFileSize)
	}

	// Check extension support.
	ext := strings.ToLower(filepath.Ext(path))
	if !idx.registry.Supported(ext) {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("unsupported extension %q for %q", ext, path)
	}

	// Compute file hash.
	hash, err := fileHash(path)
	if err != nil {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("hashing %q: %w", path, err)
	}

	// Check existing document — skip if same hash.
	existing, err := idx.store.GetDocumentByPath(ctx, path)
	switch {
	case err == nil && existing != nil:
		if existing.FileHash == hash && existing.Status == StatusIndexed {
			idx.logger.Debug("file unchanged, skipping", zap.String("path", path))
			idx.recordIndexed(trackProgress)
			return nil
		}
		// File changed — remove old data.
		if err := idx.removeDocumentData(ctx, existing); err != nil {
			idx.recordFailed(trackProgress)
			return fmt.Errorf("removing old data for %q: %w", path, err)
		}
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		idx.recordFailed(trackProgress)
		return fmt.Errorf("looking up existing document for %q: %w", path, err)
	}

	// Create document record.
	doc := &Document{
		Path:      path,
		Filename:  filepath.Base(path),
		Extension: ext,
		SizeBytes: info.Size(),
		FileHash:  hash,
		Status:    StatusIndexing,
	}
	if err := idx.store.CreateDocument(ctx, doc); err != nil {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("creating document record: %w", err)
	}

	// Set status=indexing.
	if err := idx.store.UpdateDocumentStatus(ctx, doc.ID, StatusIndexing, ""); err != nil {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("setting indexing status: %w", err)
	}

	// Parse.
	parser := idx.registry.Get(ext)
	result, err := parser.Parse(path)
	if err != nil {
		idx.markError(ctx, doc.ID, err, trackProgress)
		return fmt.Errorf("parsing %q: %w", path, err)
	}

	// Chunk.
	chunkOutputs := idx.chunker.Chunk(result)
	if len(chunkOutputs) == 0 {
		if err := idx.store.UpdateDocumentStatus(ctx, doc.ID, StatusIndexed, ""); err != nil {
			idx.recordFailed(trackProgress)
			return fmt.Errorf("setting indexed status: %w", err)
		}
		idx.recordIndexed(trackProgress)
		return nil
	}

	// Build chunk records.
	chunks := make([]Chunk, len(chunkOutputs))
	for i, co := range chunkOutputs {
		chunks[i] = Chunk{
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    co.Content,
			StartLine:  co.StartLine,
			EndLine:    co.EndLine,
			Metadata:   metadataJSON(co.Metadata),
		}
	}

	// Store chunks in DB.
	if err := idx.store.CreateChunks(ctx, chunks); err != nil {
		idx.markError(ctx, doc.ID, err, trackProgress)
		return fmt.Errorf("storing chunks: %w", err)
	}

	// Embed and add to vector index.
	for i := range chunks {
		vec, err := idx.embedder.Embed(ctx, chunks[i].Content)
		if err != nil {
			idx.markError(ctx, doc.ID, err, trackProgress)
			return fmt.Errorf("embedding chunk %d: %w", i, err)
		}
		if err := idx.index.Add(chunks[i].ID, vec); err != nil {
			idx.markError(ctx, doc.ID, err, trackProgress)
			return fmt.Errorf("adding chunk %d to vector index: %w", i, err)
		}

		// Index in bleve if available.
		if idx.bleve != nil {
			if err := idx.bleve.IndexChunk(chunks[i], doc.Path); err != nil {
				idx.logger.Warn("bleve indexing failed", zap.String("chunk", chunks[i].ID), zap.Error(err))
			}
		}
	}

	// Mark indexed.
	if err := idx.store.UpdateDocumentStatus(ctx, doc.ID, StatusIndexed, ""); err != nil {
		idx.recordFailed(trackProgress)
		return fmt.Errorf("setting indexed status: %w", err)
	}

	idx.recordIndexed(trackProgress)

	idx.logger.Info("file indexed",
		zap.String("path", path),
		zap.Int("chunks", len(chunks)))

	return nil
}

// RemoveFile removes a file from the index, cleaning up vectors and bleve.
func (idx *Indexer) RemoveFile(ctx context.Context, path string) error {
	doc, err := idx.store.GetDocumentByPath(ctx, path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("getting document for %q: %w", path, err)
	}
	if doc == nil {
		return nil
	}

	return idx.removeDocumentData(ctx, doc)
}

// RebuildAll re-indexes all qualifying files in the given paths.
func (idx *Indexer) RebuildAll(ctx context.Context, paths []string) error {
	expandedRoots := make([]string, 0, len(paths))
	for _, root := range paths {
		expandedRoots = append(expandedRoots, filepath.Clean(expandHome(root)))
	}

	filePaths := idx.collectIndexablePaths(expandedRoots)
	if idx.progress != nil {
		idx.progress.Begin("rebuilding", len(filePaths))
		defer idx.progress.Finish()
	}

	seen := make(map[string]struct{}, len(filePaths))
	for _, path := range filePaths {
		seen[path] = struct{}{}
		if err := idx.BatchIndexFile(ctx, path); err != nil {
			idx.logger.Warn("indexing failed", zap.String("path", path), zap.Error(err))
		}
	}

	docs, err := idx.store.ListDocuments(ctx)
	if err != nil {
		return fmt.Errorf("listing indexed documents for rebuild: %w", err)
	}
	for i := range docs {
		doc := docs[i]
		if _, ok := seen[doc.Path]; ok {
			continue
		}
		if !pathUnderRoots(doc.Path, expandedRoots) {
			continue
		}
		if err := idx.removeDocumentData(ctx, &doc); err != nil {
			return fmt.Errorf("removing stale document %q: %w", doc.Path, err)
		}
	}
	return nil
}

// removeDocumentData removes vectors, bleve entries, chunks, and the document.
func (idx *Indexer) removeDocumentData(ctx context.Context, doc *Document) error {
	// Remove chunk vectors and bleve entries.
	chunkIDs, err := idx.store.ChunkIDsByDocumentID(ctx, doc.ID)
	if err != nil {
		return fmt.Errorf("getting chunk IDs: %w", err)
	}
	for _, cid := range chunkIDs {
		if err := idx.index.Delete(cid); err != nil {
			idx.logger.Warn("vector delete failed", zap.String("chunk", cid), zap.Error(err))
		}
		if idx.bleve != nil {
			if err := idx.bleve.RemoveChunk(cid); err != nil {
				idx.logger.Warn("bleve delete failed", zap.String("chunk", cid), zap.Error(err))
			}
		}
	}

	// Delete document (cascades to chunks).
	if err := idx.store.DeleteDocument(ctx, doc.ID); err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}

	idx.logger.Debug("document removed", zap.String("path", doc.Path))
	return nil
}

func (idx *Indexer) markError(ctx context.Context, docID string, origErr error, trackProgress bool) {
	if err := idx.store.UpdateDocumentStatus(ctx, docID, StatusError, origErr.Error()); err != nil {
		idx.logger.Error("failed to mark document error", zap.String("doc", docID), zap.Error(err))
	}
	idx.recordFailed(trackProgress)
}

func (idx *Indexer) shouldIgnoreDir(name string) bool {
	if idx.cfg == nil {
		return false
	}
	for _, d := range idx.cfg.IgnoreDirs {
		if d == name {
			return true
		}
	}
	return false
}

func (idx *Indexer) isAllowedExtension(ext string) bool {
	if idx.cfg == nil || len(idx.cfg.Extensions) == 0 {
		return true
	}
	for _, e := range idx.cfg.Extensions {
		if strings.EqualFold(e, ext) {
			return true
		}
	}
	return false
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (idx *Indexer) recordIndexed(trackProgress bool) {
	if trackProgress && idx.progress != nil {
		idx.progress.RecordIndexed()
	}
}

func (idx *Indexer) recordFailed(trackProgress bool) {
	if trackProgress && idx.progress != nil {
		idx.progress.RecordFailed()
	}
}

func (idx *Indexer) collectIndexablePaths(roots []string) []string {
	var paths []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if idx.shouldIgnoreDir(d.Name()) && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if idx.CanIndex(path) {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			idx.logger.Warn("walk failed", zap.String("root", root), zap.Error(err))
		}
	}

	sort.Strings(paths)
	return paths
}

func pathUnderRoots(path string, roots []string) bool {
	cleanPath := filepath.Clean(path)
	for _, root := range roots {
		rel, err := filepath.Rel(root, cleanPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
