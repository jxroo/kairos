package inference

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// --- mock implementations -------------------------------------------------

type mockMemorySearcher struct {
	results []MemoryResult
	err     error
}

func (m *mockMemorySearcher) Search(_ context.Context, _ string, _ int) ([]MemoryResult, error) {
	return m.results, m.err
}

type mockRAGSearcher struct {
	results []RAGResult
	err     error
}

func (r *mockRAGSearcher) Search(_ context.Context, _ string, _ int) ([]RAGResult, error) {
	return r.results, r.err
}

// --- helpers ---------------------------------------------------------------

func newNopLogger() *zap.Logger { return zap.NewNop() }

// makeMessages builds a simple message slice with the given roles/contents.
func makeMessages(pairs ...string) []Message {
	if len(pairs)%2 != 0 {
		panic("makeMessages requires an even number of arguments (role, content, ...)")
	}
	msgs := make([]Message, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		msgs = append(msgs, Message{Role: pairs[i], Content: pairs[i+1]})
	}
	return msgs
}

// --- TestContextAssemble ---------------------------------------------------

func TestContextAssemble(t *testing.T) {
	tests := []struct {
		name          string
		memorySvc     MemorySearcher
		ragSvc        RAGSearcher
		inputMessages []Message
		opts          AssembleOpts
		// verification functions
		checkFn func(t *testing.T, got []Message)
	}{
		{
			name:      "both services nil — messages pass through unchanged",
			memorySvc: nil,
			ragSvc:    nil,
			inputMessages: makeMessages(
				"user", "Hello world",
			),
			opts: AssembleOpts{SystemPrompt: "You are helpful."},
			checkFn: func(t *testing.T, got []Message) {
				// System message should be prepended.
				if len(got) < 2 {
					t.Fatalf("expected at least 2 messages, got %d", len(got))
				}
				if got[0].Role != "system" {
					t.Errorf("first message role = %q, want %q", got[0].Role, "system")
				}
				if !strings.Contains(got[0].Content, "You are helpful.") {
					t.Errorf("system content missing system prompt: %q", got[0].Content)
				}
				// No memory/RAG sections.
				if strings.Contains(got[0].Content, "Relevant Memories") {
					t.Error("unexpected 'Relevant Memories' section with nil memory service")
				}
				if strings.Contains(got[0].Content, "Relevant Documents") {
					t.Error("unexpected 'Relevant Documents' section with nil RAG service")
				}
			},
		},
		{
			name: "memory results injected into system prompt",
			memorySvc: &mockMemorySearcher{
				results: []MemoryResult{
					{Content: "User likes Go.", Score: 0.9},
					{Content: "User prefers dark mode.", Score: 0.8},
				},
			},
			ragSvc: nil,
			inputMessages: makeMessages(
				"user", "Tell me about preferences",
			),
			opts: AssembleOpts{SystemPrompt: "Base prompt."},
			checkFn: func(t *testing.T, got []Message) {
				if got[0].Role != "system" {
					t.Fatalf("first message role = %q, want system", got[0].Role)
				}
				sys := got[0].Content
				if !strings.Contains(sys, "Relevant Memories") {
					t.Error("expected 'Relevant Memories' section in system message")
				}
				if !strings.Contains(sys, "User likes Go.") {
					t.Error("expected first memory in system message")
				}
				if !strings.Contains(sys, "User prefers dark mode.") {
					t.Error("expected second memory in system message")
				}
				if strings.Contains(sys, "Relevant Documents") {
					t.Error("unexpected 'Relevant Documents' with nil RAG service")
				}
			},
		},
		{
			name:      "RAG results injected into system prompt",
			memorySvc: nil,
			ragSvc: &mockRAGSearcher{
				results: []RAGResult{
					{Content: "Kairos is written in Go.", Source: "README.md", Score: 0.85},
				},
			},
			inputMessages: makeMessages(
				"user", "What is Kairos?",
			),
			opts: AssembleOpts{SystemPrompt: "Base prompt."},
			checkFn: func(t *testing.T, got []Message) {
				sys := got[0].Content
				if !strings.Contains(sys, "Relevant Documents") {
					t.Error("expected 'Relevant Documents' section")
				}
				if !strings.Contains(sys, "[README.md,") {
					t.Error("expected source in RAG entry")
				}
				if !strings.Contains(sys, "Kairos is written in Go.") {
					t.Error("expected RAG content in system message")
				}
				if strings.Contains(sys, "Relevant Memories") {
					t.Error("unexpected 'Relevant Memories' with nil memory service")
				}
			},
		},
		{
			name: "both memory and RAG injected",
			memorySvc: &mockMemorySearcher{
				results: []MemoryResult{
					{Content: "Memory A.", Score: 0.9},
				},
			},
			ragSvc: &mockRAGSearcher{
				results: []RAGResult{
					{Content: "Doc A content.", Source: "doc_a.md", Score: 0.8},
				},
			},
			inputMessages: makeMessages(
				"user", "Some question",
			),
			opts: AssembleOpts{SystemPrompt: "System."},
			checkFn: func(t *testing.T, got []Message) {
				sys := got[0].Content
				if !strings.Contains(sys, "Relevant Memories") {
					t.Error("expected 'Relevant Memories' section")
				}
				if !strings.Contains(sys, "Memory A.") {
					t.Error("expected memory content")
				}
				if !strings.Contains(sys, "Relevant Documents") {
					t.Error("expected 'Relevant Documents' section")
				}
				if !strings.Contains(sys, "Doc A content.") {
					t.Error("expected RAG document content")
				}
			},
		},
		{
			name:      "empty query — no user messages — returns unchanged",
			memorySvc: &mockMemorySearcher{results: []MemoryResult{{Content: "mem", Score: 1.0}}},
			ragSvc:    &mockRAGSearcher{results: []RAGResult{{Content: "doc", Source: "f.md", Score: 1.0}}},
			inputMessages: makeMessages(
				"assistant", "How can I help?",
			),
			opts: AssembleOpts{SystemPrompt: "System."},
			checkFn: func(t *testing.T, got []Message) {
				if len(got) != 1 {
					t.Fatalf("expected 1 message, got %d", len(got))
				}
				if got[0].Role != "assistant" {
					t.Errorf("role = %q, want assistant", got[0].Role)
				}
			},
		},
		{
			name:          "nil input messages — no user messages — returns nil",
			memorySvc:     nil,
			ragSvc:        nil,
			inputMessages: nil,
			opts:          AssembleOpts{SystemPrompt: "System."},
			checkFn: func(t *testing.T, got []Message) {
				if len(got) != 0 {
					t.Fatalf("expected empty result, got %d messages", len(got))
				}
			},
		},
		{
			name: "memory search error — graceful degradation",
			memorySvc: &mockMemorySearcher{
				err: errors.New("memory backend unavailable"),
			},
			ragSvc: &mockRAGSearcher{
				results: []RAGResult{
					{Content: "Doc content.", Source: "file.go", Score: 0.7},
				},
			},
			inputMessages: makeMessages(
				"user", "question",
			),
			opts: AssembleOpts{SystemPrompt: "Prompt."},
			checkFn: func(t *testing.T, got []Message) {
				// Assembly must succeed.
				if len(got) == 0 {
					t.Fatal("expected at least 1 message")
				}
				sys := got[0].Content
				// RAG results should still be present.
				if !strings.Contains(sys, "Doc content.") {
					t.Error("expected RAG content even when memory search failed")
				}
				// No memory section.
				if strings.Contains(sys, "Relevant Memories") {
					t.Error("unexpected 'Relevant Memories' when memory search errored")
				}
			},
		},
		{
			name: "RAG search error — graceful degradation",
			memorySvc: &mockMemorySearcher{
				results: []MemoryResult{
					{Content: "Memory content.", Score: 0.9},
				},
			},
			ragSvc: &mockRAGSearcher{
				err: errors.New("RAG backend unavailable"),
			},
			inputMessages: makeMessages(
				"user", "question",
			),
			opts: AssembleOpts{SystemPrompt: "Prompt."},
			checkFn: func(t *testing.T, got []Message) {
				sys := got[0].Content
				// Memory section should be present.
				if !strings.Contains(sys, "Memory content.") {
					t.Error("expected memory content even when RAG search failed")
				}
				if strings.Contains(sys, "Relevant Documents") {
					t.Error("unexpected 'Relevant Documents' when RAG search errored")
				}
			},
		},
		{
			name:      "existing system message at index 0 is replaced",
			memorySvc: nil,
			ragSvc:    nil,
			inputMessages: makeMessages(
				"system", "Old system message.",
				"user", "User question.",
			),
			opts: AssembleOpts{SystemPrompt: "New system prompt."},
			checkFn: func(t *testing.T, got []Message) {
				if got[0].Role != "system" {
					t.Fatalf("first role = %q, want system", got[0].Role)
				}
				if strings.Contains(got[0].Content, "Old system message.") {
					t.Error("old system message should have been replaced")
				}
				if !strings.Contains(got[0].Content, "New system prompt.") {
					t.Error("expected new system prompt in system message")
				}
				// User message preserved.
				if got[1].Role != "user" || got[1].Content != "User question." {
					t.Errorf("user message not preserved: %+v", got[1])
				}
			},
		},
		{
			name:      "system prompt always preserved through truncation",
			memorySvc: nil,
			ragSvc:    nil,
			// Build a long conversation that would exceed a small budget.
			inputMessages: func() []Message {
				msgs := make([]Message, 0, 22)
				for i := 0; i < 10; i++ {
					msgs = append(msgs,
						Message{Role: "user", Content: strings.Repeat("word ", 80)},
						Message{Role: "assistant", Content: strings.Repeat("reply ", 80)},
					)
				}
				msgs = append(msgs, Message{Role: "user", Content: "Final question"})
				return msgs
			}(),
			opts: AssembleOpts{
				SystemPrompt:   "Critical system instructions.",
				MaxTokens:      400, // tight budget
				ReservedTokens: 1,   // minimal reserve
			},
			checkFn: func(t *testing.T, got []Message) {
				if len(got) == 0 {
					t.Fatal("expected at least 1 message")
				}
				if got[0].Role != "system" {
					t.Fatalf("first role = %q, want system", got[0].Role)
				}
				if !strings.Contains(got[0].Content, "Critical system instructions.") {
					t.Error("system prompt must always be preserved")
				}
			},
		},
		{
			name:      "truncation drops oldest messages first",
			memorySvc: nil,
			ragSvc:    nil,
			inputMessages: func() []Message {
				// 5 older turns + 1 final user question.
				msgs := []Message{
					{Role: "user", Content: strings.Repeat("old1 ", 100)},      // ~125 tokens
					{Role: "assistant", Content: strings.Repeat("old2 ", 100)}, // ~125 tokens
					{Role: "user", Content: strings.Repeat("old3 ", 100)},      // ~125 tokens
					{Role: "assistant", Content: strings.Repeat("old4 ", 100)}, // ~125 tokens
					{Role: "user", Content: "Recent question"},                 // tiny
				}
				return msgs
			}(),
			opts: AssembleOpts{
				SystemPrompt:   "sys",
				MaxTokens:      500,
				ReservedTokens: 1, // minimal reserve to keep effective budget usable
			},
			checkFn: func(t *testing.T, got []Message) {
				// System message present.
				if got[0].Role != "system" {
					t.Fatalf("first role = %q, want system", got[0].Role)
				}
				// Last user message preserved.
				last := got[len(got)-1]
				if last.Role != "user" || last.Content != "Recent question" {
					t.Errorf("last message = %+v, expected final user message", last)
				}
				// The oldest messages (old1, old2) should have been dropped.
				for _, m := range got {
					if strings.Contains(m.Content, "old1") {
						t.Error("oldest message 'old1' should have been dropped")
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assembler := NewContextAssembler(tc.memorySvc, tc.ragSvc, newNopLogger())
			got, err := assembler.Assemble(context.Background(), tc.inputMessages, tc.opts)
			if err != nil {
				t.Fatalf("Assemble returned unexpected error: %v", err)
			}
			tc.checkFn(t, got)
		})
	}
}

func TestContextAssembleAlwaysFitsTokenBudget(t *testing.T) {
	assembler := NewContextAssembler(nil, nil, newNopLogger())
	messages := []Message{
		{Role: "user", Content: strings.Repeat("u", 120)},
		{Role: "assistant", Content: strings.Repeat("a", 120)},
		{Role: "user", Content: strings.Repeat("b", 120)},
	}

	// Use ReservedTokens=0 explicitly so the effective budget stays small.
	// MaxTokens large enough to allow at least system + last user msg after trimming.
	got, err := assembler.Assemble(context.Background(), messages, AssembleOpts{
		SystemPrompt:   strings.Repeat("s", 120),
		MaxTokens:      100,
		ReservedTokens: 1, // minimal reserve to avoid default 512
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	// Effective budget = 100 - 1 = 99. All messages should fit within that after trimming.
	effectiveBudget := 99
	if totalTokens(got) > effectiveBudget {
		t.Fatalf("total tokens = %d, want <= %d", totalTokens(got), effectiveBudget)
	}
	if len(got) == 0 || got[0].Role != "system" {
		t.Fatalf("expected system message to be preserved, got %+v", got)
	}
	if got[len(got)-1].Role != "user" {
		t.Fatalf("expected last user message to be preserved, got %+v", got)
	}
}

// TestContextAssembleDefaults verifies that zero-value MemoryLimit/RAGLimit
// are treated as their defaults (5 and 3 respectively) and that the mock is
// called with those values.
func TestContextAssembleDefaults(t *testing.T) {
	var capturedMemLimit, capturedRAGLimit int

	mem := &capturingMemorySearcher{captureLimit: &capturedMemLimit}
	rag := &capturingRAGSearcher{captureLimit: &capturedRAGLimit}

	a := NewContextAssembler(mem, rag, newNopLogger())
	_, err := a.Assemble(context.Background(),
		makeMessages("user", "hello"),
		AssembleOpts{SystemPrompt: "s"}, // zero MemoryLimit / RAGLimit
	)
	if err != nil {
		t.Fatalf("Assemble error: %v", err)
	}

	if capturedMemLimit != defaultMemoryLimit {
		t.Errorf("memory limit = %d, want %d", capturedMemLimit, defaultMemoryLimit)
	}
	if capturedRAGLimit != defaultRAGLimit {
		t.Errorf("RAG limit = %d, want %d", capturedRAGLimit, defaultRAGLimit)
	}
}

// capturingMemorySearcher records the limit passed to Search.
type capturingMemorySearcher struct {
	captureLimit *int
}

func (c *capturingMemorySearcher) Search(_ context.Context, _ string, limit int) ([]MemoryResult, error) {
	*c.captureLimit = limit
	return nil, nil
}

// capturingRAGSearcher records the limit passed to Search.
type capturingRAGSearcher struct {
	captureLimit *int
}

func (c *capturingRAGSearcher) Search(_ context.Context, _ string, limit int) ([]RAGResult, error) {
	*c.captureLimit = limit
	return nil, nil
}

// TestBuildSystemContent verifies formatting edge-cases.
func TestBuildSystemContent(t *testing.T) {
	tests := []struct {
		name         string
		systemPrompt string
		memories     []MemoryResult
		ragResults   []RAGResult
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "no memories no rag — only system prompt",
			systemPrompt: "Base.",
			wantContains: []string{"Base."},
			wantAbsent:   []string{"Relevant Memories", "Relevant Documents"},
		},
		{
			name:         "only memories",
			systemPrompt: "Prompt.",
			memories:     []MemoryResult{{Content: "mem1", Score: 1.0}},
			wantContains: []string{"Relevant Memories", "mem1"},
			wantAbsent:   []string{"Relevant Documents"},
		},
		{
			name:         "only rag",
			systemPrompt: "Prompt.",
			ragResults:   []RAGResult{{Content: "chunk1", Source: "file.go", Score: 1.0}},
			wantContains: []string{"Relevant Documents", "[file.go,", "chunk1"},
			wantAbsent:   []string{"Relevant Memories"},
		},
		{
			name:         "both memories and rag",
			systemPrompt: "Prompt.",
			memories:     []MemoryResult{{Content: "mem1", Score: 1.0}},
			ragResults:   []RAGResult{{Content: "chunk1", Source: "file.go", Score: 1.0}},
			wantContains: []string{"Relevant Memories", "mem1", "Relevant Documents", "[file.go,", "chunk1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSystemContent(tc.systemPrompt, tc.memories, tc.ragResults)
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in output:\n%s", want, got)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("unexpected %q in output:\n%s", absent, got)
				}
			}
		})
	}
}

// TestEstimateTokens verifies the chars/4 heuristic plus per-message overhead.
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want int
	}{
		{"empty content", Message{Role: "user", Content: ""}, perMessageOverhead},
		{"4 chars", Message{Role: "user", Content: "abcd"}, perMessageOverhead + 1},
		{"5 chars", Message{Role: "user", Content: "abcde"}, perMessageOverhead + 1},
		{"8 chars", Message{Role: "user", Content: "abcdefgh"}, perMessageOverhead + 2},
		{"400 chars", Message{Role: "user", Content: strings.Repeat("x", 400)}, perMessageOverhead + 100},
		{
			"with tool calls",
			Message{
				Role:    "assistant",
				Content: "Let me help.",
				ToolCalls: []ToolCall{
					{
						ID:   "call_1234567890ab", // 20 chars → 5 tokens
						Type: "function",           // 8 chars → 2 tokens
						Function: FunctionCall{
							Name:      "search_memory",                   // 13 chars → 3 tokens
							Arguments: `{"query":"test","limit":5}`, // 26 chars → 6 tokens
						},
					},
				},
			},
			// overhead(4) + content(12/4=3) + ID(18/4=4) + type(8/4=2) + name(13/4=3) + args(26/4=6) = 22
			perMessageOverhead + 3 + 4 + 2 + 3 + 6,
		},
		{
			"with tool call ID",
			Message{
				Role:       "tool",
				Content:    "result data here",
				ToolCallID: "call_1234567890ab", // 20 chars → 5 tokens
			},
			perMessageOverhead + 4 + 4, // overhead + content(16/4) + toolCallID(18/4)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := estimateTokens(tc.msg)
			if got != tc.want {
				t.Errorf("estimateTokens() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestEstimateToolTokens verifies token cost estimation for tool definitions.
func TestEstimateToolTokens(t *testing.T) {
	t.Run("nil specs returns 0", func(t *testing.T) {
		if got := EstimateToolTokens(nil); got != 0 {
			t.Errorf("EstimateToolTokens(nil) = %d, want 0", got)
		}
	})

	t.Run("empty specs returns 0", func(t *testing.T) {
		if got := EstimateToolTokens([]ToolSpec{}); got != 0 {
			t.Errorf("EstimateToolTokens([]) = %d, want 0", got)
		}
	})

	t.Run("single tool", func(t *testing.T) {
		specs := []ToolSpec{
			{
				Type: "function",
				Function: ToolFunctionSpec{
					Name:        "search_memory",
					Description: "Search through stored memories",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
			},
		}
		got := EstimateToolTokens(specs)
		if got <= 0 {
			t.Errorf("EstimateToolTokens() = %d, want > 0", got)
		}
	})

	t.Run("more tools = more tokens", func(t *testing.T) {
		oneTool := []ToolSpec{
			{Type: "function", Function: ToolFunctionSpec{Name: "a", Description: "does a"}},
		}
		twoTools := []ToolSpec{
			{Type: "function", Function: ToolFunctionSpec{Name: "a", Description: "does a"}},
			{Type: "function", Function: ToolFunctionSpec{Name: "b", Description: "does b"}},
		}
		one := EstimateToolTokens(oneTool)
		two := EstimateToolTokens(twoTools)
		if two <= one {
			t.Errorf("two tools (%d) should cost more than one (%d)", two, one)
		}
	})
}

// TestAssembleWithReservedTokens verifies that ReservedTokens and ToolOverheadTokens
// reduce the effective budget used for context assembly.
func TestAssembleWithReservedTokens(t *testing.T) {
	mem := &mockMemorySearcher{
		results: []MemoryResult{
			{Content: strings.Repeat("memory content ", 20), Score: 0.9},
		},
	}
	rag := &mockRAGSearcher{
		results: []RAGResult{
			{Content: strings.Repeat("document chunk ", 20), Source: "doc.md", Score: 0.8},
		},
	}

	assembler := NewContextAssembler(mem, rag, newNopLogger())

	// With a generous budget, enrichment should be included.
	gotGenerous, err := assembler.Assemble(context.Background(),
		makeMessages("user", "hello"),
		AssembleOpts{
			SystemPrompt:   "sys",
			MaxTokens:      2000,
			ReservedTokens: 100,
		},
	)
	if err != nil {
		t.Fatalf("Assemble (generous): %v", err)
	}

	// With a tight budget after reserves, enrichment may be reduced/skipped.
	gotTight, err := assembler.Assemble(context.Background(),
		makeMessages("user", "hello"),
		AssembleOpts{
			SystemPrompt:       "sys",
			MaxTokens:          200,
			ReservedTokens:     150,
			ToolOverheadTokens: 30,
		},
	)
	if err != nil {
		t.Fatalf("Assemble (tight): %v", err)
	}

	// The generous result should have more content than the tight one.
	generousTokens := totalTokens(gotGenerous)
	tightTokens := totalTokens(gotTight)
	if generousTokens <= tightTokens {
		t.Errorf("generous tokens (%d) should exceed tight tokens (%d)", generousTokens, tightTokens)
	}

	// The tight result should respect effective budget: MaxTokens - ReservedTokens - ToolOverheadTokens = 20
	effectiveBudget := 200 - 150 - 30
	if tightTokens > effectiveBudget {
		t.Errorf("tight total tokens (%d) exceeds effective budget (%d)", tightTokens, effectiveBudget)
	}
}

// TestAssembleProactiveSkipsSearch verifies that when the conversation already
// fills the effective budget, memory and RAG searches are skipped entirely.
func TestAssembleProactiveSkipsSearch(t *testing.T) {
	memCalled := false
	ragCalled := false

	mem := &trackingMemorySearcher{called: &memCalled}
	rag := &trackingRAGSearcher{called: &ragCalled}

	assembler := NewContextAssembler(mem, rag, newNopLogger())

	// Create a conversation that already consumes most of the budget.
	// Each message: perMessageOverhead(4) + content/4. With 200-char content → 50+4=54 tokens.
	longContent := strings.Repeat("x", 200)
	messages := makeMessages(
		"user", longContent,
		"assistant", longContent,
		"user", longContent,
	)
	// 3 messages × ~54 tokens = ~162 tokens for conversation alone.
	// System prompt "sys" = 4 + 0 = ~4 tokens.
	// Total ~166 tokens. With MaxTokens=200, ReservedTokens=50 → effective=150.
	// Available for enrichment = 150 - 166 < 0 → should skip.

	got, err := assembler.Assemble(context.Background(), messages, AssembleOpts{
		SystemPrompt:   "sys",
		MaxTokens:      200,
		ReservedTokens: 50,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if memCalled {
		t.Error("memory search should have been skipped due to tight budget")
	}
	if ragCalled {
		t.Error("RAG search should have been skipped due to tight budget")
	}

	// System message should still be present (without enrichment sections).
	if len(got) == 0 || got[0].Role != "system" {
		t.Fatal("expected system message to be present")
	}
	if strings.Contains(got[0].Content, "Relevant Memories") {
		t.Error("unexpected enrichment in system message when budget was exhausted")
	}
}

// trackingMemorySearcher records whether Search was called.
type trackingMemorySearcher struct {
	called *bool
}

func (t *trackingMemorySearcher) Search(_ context.Context, _ string, _ int) ([]MemoryResult, error) {
	*t.called = true
	return []MemoryResult{{Content: "tracked memory", Score: 0.9}}, nil
}

// trackingRAGSearcher records whether Search was called.
type trackingRAGSearcher struct {
	called *bool
}

func (t *trackingRAGSearcher) Search(_ context.Context, _ string, _ int) ([]RAGResult, error) {
	*t.called = true
	return []RAGResult{{Content: "tracked doc", Source: "f.md", Score: 0.8}}, nil
}

func TestSearchQuery(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     string
	}{
		{
			name:     "no user messages",
			messages: makeMessages("assistant", "Hello"),
			want:     "",
		},
		{
			name:     "single long user message",
			messages: makeMessages("user", "What is the meaning of life and the universe?"),
			want:     "What is the meaning of life and the universe?",
		},
		{
			name:     "single short user message — no augmentation possible",
			messages: makeMessages("user", "yes"),
			want:     "yes",
		},
		{
			name: "short follow-up augmented with previous",
			messages: makeMessages(
				"user", "Tell me about the Kairos project architecture",
				"assistant", "Kairos is a Go daemon...",
				"user", "tell me more",
			),
			want: "Tell me about the Kairos project architecture tell me more",
		},
		{
			name: "long follow-up not augmented",
			messages: makeMessages(
				"user", "Tell me about the Kairos project",
				"assistant", "Kairos is a Go daemon...",
				"user", "How does the memory search work in detail?",
			),
			want: "How does the memory search work in detail?",
		},
		{
			name: "previous message truncated to 200 chars",
			messages: makeMessages(
				"user", strings.Repeat("x", 300),
				"assistant", "response",
				"user", "yes",
			),
			want: strings.Repeat("x", 200) + " yes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := searchQuery(tc.messages)
			if got != tc.want {
				t.Errorf("searchQuery() = %q, want %q", got, tc.want)
			}
		})
	}
}
