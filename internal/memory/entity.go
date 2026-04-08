package memory

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EntityExtractor extracts named entities from free text using regex and
// heuristic rules. It requires no external NLP dependencies.
type EntityExtractor struct {
	logger *zap.Logger
}

// ExtractedEntity holds the name and type of an entity found in text.
type ExtractedEntity struct {
	Name string
	Type string
}

// Compiled regex patterns used by EntityExtractor.
// All patterns are package-level to avoid recompilation on every call.
var (
	// Dates: ISO 8601 (2026-03-12), long form (March 12, 2026), slash form (12/03/2026).
	reDateISO   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	reDateLong  = regexp.MustCompile(`\b(?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},?\s+\d{4}\b`)
	reDateSlash = regexp.MustCompile(`\b\d{1,2}/\d{1,2}/\d{4}\b`)

	// URLs: http or https followed by non-whitespace characters.
	reURL = regexp.MustCompile(`https?://\S+`)

	// Emails: requires at least one dot in the domain; domain components must
	// not end with a dot, preventing absorption of trailing sentence dots.
	// Pattern: local-part @ label (. label)+
	reEmail = regexp.MustCompile(`[\w.+\-]+@[\w\-]+(?:\.[\w\-]+)+`)

	// Project keywords: enumerate keyword cases explicitly so that [A-Z] in
	// the capture group is not affected by a (?i) flag.
	reProject = regexp.MustCompile(`\b(?:[Pp]roject|[Rr]epo|[Aa]pp)\s+([A-Z][A-Za-z0-9_\-]*)`)

	// reSentence splits text on sentence-ending punctuation followed by space.
	// Promoted to package-level to avoid recompilation inside extractPersons.
	reSentence = regexp.MustCompile(`[.!?]\s+`)

	// reWord tokenizes a sentence into letter-based words.
	// Promoted to package-level to avoid recompilation inside extractPersons.
	reWord = regexp.MustCompile(`[A-Za-z][A-Za-z'\-]*`)
)

// NewEntityExtractor returns an EntityExtractor that logs via logger.
func NewEntityExtractor(logger *zap.Logger) *EntityExtractor {
	return &EntityExtractor{logger: logger}
}

// Extract parses text and returns all detected entities, deduplicating by
// (Name, Type). The extraction order is: dates, URLs, emails, projects,
// then capitalized person/org sequences.
func (e *EntityExtractor) Extract(text string) []ExtractedEntity {
	seen := make(map[string]struct{})
	var results []ExtractedEntity

	add := func(name, typ string) {
		key := typ + ":" + name
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		results = append(results, ExtractedEntity{Name: name, Type: typ})
	}

	// --- dates ---------------------------------------------------------------
	for _, m := range reDateLong.FindAllString(text, -1) {
		add(m, "date")
	}
	for _, m := range reDateISO.FindAllString(text, -1) {
		add(m, "date")
	}
	for _, m := range reDateSlash.FindAllString(text, -1) {
		add(m, "date")
	}

	// --- URLs ----------------------------------------------------------------
	// Strip trailing punctuation that can be absorbed by \S+.
	// Collect raw URL match indices so we can blank out URL spans before
	// running email extraction (prevents matching credentials in URLs such as
	// user:pass@host).
	urlIdxs := reURL.FindAllStringIndex(text, -1)
	for _, m := range reURL.FindAllString(text, -1) {
		m = strings.TrimRight(m, ".,;:!?\"')")
		add(m, "url")
	}

	// --- emails --------------------------------------------------------------
	// Build a copy of the text with all URL spans replaced by spaces so that
	// the email regex cannot match inside a URL (e.g. user@host in https://user@host/path).
	emailText := blankOutSpans(text, urlIdxs)
	for _, m := range reEmail.FindAllString(emailText, -1) {
		add(m, "email")
	}

	// --- projects ------------------------------------------------------------
	for _, sub := range reProject.FindAllStringSubmatch(text, -1) {
		if len(sub) >= 2 {
			add(sub[1], "project")
		}
	}

	// --- people / organizations (capitalized sequences not at sentence start) -
	for _, name := range extractPersons(text) {
		add(name, "person")
	}

	e.logger.Debug("entity extraction complete",
		zap.String("text_preview", truncate(text, 60)),
		zap.Int("entities_found", len(results)),
	)
	return results
}

// extractPersons finds 2–3 consecutive capitalized words that do NOT appear
// at the very start of a sentence (to avoid treating sentence-opening words
// as person names). It returns deduplicated names in order of first
// occurrence. Uses package-level reSentence and reWord to avoid per-call
// compilation.
func extractPersons(text string) []string {
	sentences := reSentence.Split(text, -1)

	seen := make(map[string]struct{})
	var names []string

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		words := reWord.FindAllString(sentence, -1)
		if len(words) == 0 {
			continue
		}

		// Determine which words are capitalized (first letter uppercase).
		caps := make([]bool, len(words))
		for i, w := range words {
			if len(w) > 0 && unicode.IsUpper(rune(w[0])) {
				caps[i] = true
			}
		}

		// Skip index 0 in each sentence — sentence-initial words are excluded.
		i := 1
		for i < len(words) {
			if !caps[i] {
				i++
				continue
			}
			// Try to extend a run of capitalized words (2–3).
			j := i + 1
			for j < len(words) && j < i+3 && caps[j] {
				j++
			}
			runLen := j - i
			if runLen >= 2 {
				name := strings.Join(words[i:j], " ")
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					names = append(names, name)
				}
				i = j
			} else {
				i++
			}
		}
	}
	return names
}

// truncate returns at most n runes of s for log messages.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// blankOutSpans returns a copy of s where each byte range [lo, hi) identified
// by spans (as returned by regexp.FindAllStringIndex) is replaced with spaces.
// The resulting string has the same byte length as s, preserving all other
// offsets so that subsequent regex matches remain valid.
func blankOutSpans(s string, spans [][]int) string {
	if len(spans) == 0 {
		return s
	}
	b := []byte(s)
	for _, span := range spans {
		for i := span[0]; i < span[1] && i < len(b); i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

// likeSafeReplacer escapes LIKE metacharacters (% and _) in entity names so
// that a partial-match LIKE query treats them literally.
var likeSafeReplacer = strings.NewReplacer(`%`, `\%`, `_`, `\_`)

// ---- Store entity methods --------------------------------------------------

// linkEntities extracts entities from content and persists them within tx,
// linking them to memoryID. Duplicate entities (same name+type) are merged
// by incrementing their mention_count.
func (s *Store) linkEntities(ctx context.Context, tx *sql.Tx, memoryID string, entities []ExtractedEntity) error {
	for _, e := range entities {
		// Try to find an existing entity by name + type.
		var entityID string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM entities WHERE name = ? AND entity_type = ?`,
			e.Name, e.Type,
		).Scan(&entityID)

		switch err {
		case nil:
			// Entity already exists — increment mention_count.
			if _, err = tx.ExecContext(ctx,
				`UPDATE entities SET mention_count = mention_count + 1 WHERE id = ?`,
				entityID,
			); err != nil {
				return fmt.Errorf("incrementing mention_count for entity %q: %w", e.Name, err)
			}
		case sql.ErrNoRows:
			// New entity — insert it.
			entityID = uuid.New().String()
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO entities (id, name, entity_type) VALUES (?, ?, ?)`,
				entityID, e.Name, e.Type,
			); err != nil {
				return fmt.Errorf("inserting entity %q: %w", e.Name, err)
			}
		default:
			return fmt.Errorf("looking up entity %q: %w", e.Name, err)
		}

		// Link entity to memory (ignore if link already exists).
		if _, err = tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO memory_entities (memory_id, entity_id) VALUES (?, ?)`,
			memoryID, entityID,
		); err != nil {
			return fmt.Errorf("linking entity %q to memory %q: %w", e.Name, memoryID, err)
		}
	}
	return nil
}

// GetEntities returns all entities linked to the given memoryID.
func (s *Store) GetEntities(ctx context.Context, memoryID string) ([]Entity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.name, e.entity_type, e.mention_count
		FROM entities e
		JOIN memory_entities me ON me.entity_id = e.id
		WHERE me.memory_id = ?
		ORDER BY e.name`, memoryID)
	if err != nil {
		return nil, fmt.Errorf("querying entities for memory %q: %w", memoryID, err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var en Entity
		if err := rows.Scan(&en.ID, &en.Name, &en.Type, &en.MentionCount); err != nil {
			return nil, fmt.Errorf("scanning entity row: %w", err)
		}
		entities = append(entities, en)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating entity rows: %w", err)
	}
	return entities, nil
}

// SearchByEntity returns all memories whose linked entities match entityName
// (case-insensitive partial match). LIKE metacharacters in entityName are
// escaped so they are treated literally.
func (s *Store) SearchByEntity(ctx context.Context, entityName string) ([]Memory, error) {
	escaped := likeSafeReplacer.Replace(entityName)
	pattern := "%" + escaped + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT m.id, m.content, COALESCE(m.summary,''), m.importance,
		       m.importance_weight, m.decay_score,
		       COALESCE(m.conversation_id,''), COALESCE(m.source,''),
		       m.created_at, m.updated_at, m.accessed_at, m.access_count
		FROM memories m
		JOIN memory_entities me ON me.memory_id = m.id
		JOIN entities e ON e.id = me.entity_id
		WHERE e.name LIKE ? ESCAPE '\'
		ORDER BY m.created_at DESC`, pattern)
	if err != nil {
		return nil, fmt.Errorf("searching memories by entity %q: %w", entityName, err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning memory row: %w", err)
		}
		tags, err := s.loadTags(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		m.Tags = tags
		memories = append(memories, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating memory rows: %w", err)
	}
	return memories, nil
}
