package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// MemorySearcher abstracts memory search for context assembly.
type MemorySearcher interface {
	Search(ctx context.Context, query string, limit int) ([]MemoryResult, error)
}

// RAGSearcher abstracts RAG search for context assembly.
type RAGSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]RAGResult, error)
}

// MemoryResult represents a memory search result for context injection.
type MemoryResult struct {
	Content    string
	Score      float64
	Tags       []string
	Importance string // "low", "normal", "high"
	CreatedAt  string // "2006-01-02" formatted date
}

// RAGResult represents a RAG search result for context injection.
type RAGResult struct {
	Content string
	Source  string // filename/path
	Score   float64
}

// AssembleOpts controls how the context is assembled.
type AssembleOpts struct {
	SystemPrompt       string
	MaxTokens          int // context budget; 0 means unlimited
	MemoryLimit        int // max memories to inject (default 5)
	RAGLimit           int // max RAG chunks to inject (default 3)
	ReservedTokens     int // tokens reserved for model response (default 512)
	ToolOverheadTokens int // tokens consumed by tool definitions
}

// ContextAssembler merges memory and RAG search results into a conversation context.
type ContextAssembler struct {
	memorySvc MemorySearcher // may be nil
	ragSvc    RAGSearcher    // may be nil
	logger    *zap.Logger
}

// NewContextAssembler creates a ContextAssembler. memorySvc and ragSvc may be nil
// to disable the respective enrichment.
func NewContextAssembler(memorySvc MemorySearcher, ragSvc RAGSearcher, logger *zap.Logger) *ContextAssembler {
	return &ContextAssembler{
		memorySvc: memorySvc,
		ragSvc:    ragSvc,
		logger:    logger,
	}
}

// defaultMemoryLimit is the default number of memories injected when MemoryLimit <= 0.
const defaultMemoryLimit = 5

// defaultRAGLimit is the default number of RAG chunks injected when RAGLimit <= 0.
const defaultRAGLimit = 3

// defaultReservedTokens is the default number of tokens reserved for model response generation.
const defaultReservedTokens = 512

// perMessageOverhead accounts for role/formatting tokens per message.
const perMessageOverhead = 4

// estimateTokens approximates the token count for a message using the chars/4 heuristic,
// including per-message overhead, tool calls, and tool call IDs.
func estimateTokens(m Message) int {
	tokens := perMessageOverhead + len(m.Content)/4

	for _, tc := range m.ToolCalls {
		// ID + type + function name + arguments
		tokens += len(tc.ID)/4 + len(tc.Type)/4 + len(tc.Function.Name)/4 + len(tc.Function.Arguments)/4
	}

	if m.ToolCallID != "" {
		tokens += len(m.ToolCallID) / 4
	}

	return tokens
}

// EstimateToolTokens estimates the token cost of tool/function definitions
// that are sent alongside chat messages.
func EstimateToolTokens(specs []ToolSpec) int {
	if len(specs) == 0 {
		return 0
	}

	tokens := 0
	for _, spec := range specs {
		// type + function name + description
		tokens += len(spec.Type)/4 + len(spec.Function.Name)/4 + len(spec.Function.Description)/4

		// parameters map serialized as JSON
		if spec.Function.Parameters != nil {
			data, err := json.Marshal(spec.Function.Parameters)
			if err == nil {
				tokens += len(data) / 4
			}
		}

		// Per-tool overhead (formatting/schema wrapper)
		tokens += perMessageOverhead
	}

	return tokens
}

// totalTokens sums estimated tokens across all messages.
func totalTokens(msgs []Message) int {
	sum := 0
	for _, m := range msgs {
		sum += estimateTokens(m)
	}
	return sum
}

// minQueryLen is the character threshold below which a user message is considered
// too short for a good embedding search (e.g. "yes", "tell me more").
const minQueryLen = 20

// searchQuery extracts a useful search query from the conversation.
// If the last user message is very short (likely a follow-up), it concatenates
// the last two user messages to provide more semantic signal for embedding search.
func searchQuery(messages []Message) string {
	var lastUser, prevUser string
	found := 0
	for i := len(messages) - 1; i >= 0 && found < 2; i-- {
		if messages[i].Role == "user" {
			if found == 0 {
				lastUser = messages[i].Content
			} else {
				prevUser = messages[i].Content
			}
			found++
		}
	}

	if lastUser == "" {
		return ""
	}

	if len(lastUser) < minQueryLen && prevUser != "" {
		prev := prevUser
		if len(prev) > 200 {
			prev = prev[:200]
		}
		return prev + " " + lastUser
	}

	return lastUser
}

// buildSystemContent constructs the system message content from the base prompt
// plus optional memory and RAG context blocks.
func buildSystemContent(systemPrompt string, memories []MemoryResult, ragResults []RAGResult) string {
	var sb strings.Builder
	sb.WriteString(systemPrompt)

	if len(memories) > 0 {
		sb.WriteString("\n\n## Relevant Memories\n")
		for _, m := range memories {
			fmt.Fprintf(&sb, "- [%.0f%%", m.Score*100)
			if m.CreatedAt != "" {
				sb.WriteString(", ")
				sb.WriteString(m.CreatedAt)
			}
			if len(m.Tags) > 0 {
				sb.WriteString(", tags: ")
				sb.WriteString(strings.Join(m.Tags, "/"))
			}
			sb.WriteString("] ")
			sb.WriteString(m.Content)
			sb.WriteString("\n")
		}
	}

	if len(ragResults) > 0 {
		sb.WriteString("\n\n## Relevant Documents\n")
		for _, r := range ragResults {
			fmt.Fprintf(&sb, "- [%s, %.0f%%]: %s\n", r.Source, r.Score*100, r.Content)
		}
	}

	return sb.String()
}

func truncateContentToTokenBudget(content string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}

	maxChars := maxTokens * 4
	if len(content) <= maxChars {
		return content
	}
	if maxChars <= 3 {
		return content[:maxChars]
	}
	return content[:maxChars-3] + "..."
}

// Assemble builds the enriched message chain for an LLM request.
//
//  1. Extract the last user message as the search query.
//  2. Search memories and RAG documents.
//  3. Compose a system message (systemPrompt + context blocks).
//  4. Replace any existing system message at index 0 (or prepend a new one).
//  5. If MaxTokens > 0 and the assembled context exceeds the budget, trim
//     oldest non-essential messages until the budget is satisfied.
func (a *ContextAssembler) Assemble(ctx context.Context, messages []Message, opts AssembleOpts) ([]Message, error) {
	// Apply defaults.
	memLimit := opts.MemoryLimit
	if memLimit <= 0 {
		memLimit = defaultMemoryLimit
	}
	ragLimit := opts.RAGLimit
	if ragLimit <= 0 {
		ragLimit = defaultRAGLimit
	}
	reservedTokens := opts.ReservedTokens
	if reservedTokens <= 0 {
		reservedTokens = defaultReservedTokens
	}

	query := searchQuery(messages)
	if query == "" {
		// No user message — return unchanged.
		return messages, nil
	}

	// Compute effective budget: total context minus response reserve minus tool definitions.
	effectiveBudget := opts.MaxTokens
	if effectiveBudget > 0 {
		effectiveBudget -= reservedTokens + opts.ToolOverheadTokens
		if effectiveBudget < 0 {
			effectiveBudget = 0
		}
	}

	// Proactive budgeting: estimate current conversation + system prompt cost
	// BEFORE searching, and limit or skip enrichment if budget is tight.
	var skipEnrichment bool
	if effectiveBudget > 0 {
		// Estimate tokens for conversation messages (excluding any existing system msg).
		convMsgs := messages
		if len(convMsgs) > 0 && convMsgs[0].Role == "system" {
			convMsgs = convMsgs[1:]
		}
		convTokens := totalTokens(convMsgs)
		systemBaseTokens := perMessageOverhead + len(opts.SystemPrompt)/4

		availableForEnrichment := effectiveBudget - convTokens - systemBaseTokens
		if availableForEnrichment <= 0 {
			// No room for enrichment — skip searches entirely.
			skipEnrichment = true
			a.logger.Debug("skipping memory/RAG enrichment: no token budget available",
				zap.Int("effective_budget", effectiveBudget),
				zap.Int("conversation_tokens", convTokens),
				zap.Int("system_base_tokens", systemBaseTokens),
			)
		} else {
			// Proportionally reduce limits: 60% memory, 40% RAG.
			// Estimate ~50 tokens per memory/RAG result as a rough guide.
			const tokensPerResult = 50
			maxMemTokens := availableForEnrichment * 60 / 100
			maxRAGTokens := availableForEnrichment * 40 / 100

			proposedMemLimit := maxMemTokens / tokensPerResult
			if proposedMemLimit < memLimit {
				if proposedMemLimit <= 0 {
					proposedMemLimit = 0
				}
				memLimit = proposedMemLimit
			}

			proposedRAGLimit := maxRAGTokens / tokensPerResult
			if proposedRAGLimit < ragLimit {
				if proposedRAGLimit <= 0 {
					proposedRAGLimit = 0
				}
				ragLimit = proposedRAGLimit
			}
		}
	}

	// Search memories.
	memories := make([]MemoryResult, 0, memLimit)
	if a.memorySvc != nil && !skipEnrichment && memLimit > 0 {
		results, err := a.memorySvc.Search(ctx, query, memLimit)
		if err != nil {
			a.logger.Warn("memory search failed, continuing without memories", zap.Error(err))
		} else {
			memories = results
		}
	}

	// Search RAG documents.
	ragResults := make([]RAGResult, 0, ragLimit)
	if a.ragSvc != nil && !skipEnrichment && ragLimit > 0 {
		results, err := a.ragSvc.Search(ctx, query, ragLimit)
		if err != nil {
			a.logger.Warn("RAG search failed, continuing without documents", zap.Error(err))
		} else {
			ragResults = results
		}
	}

	// Build system message.
	systemContent := buildSystemContent(opts.SystemPrompt, memories, ragResults)
	systemMsg := Message{Role: "system", Content: systemContent}

	// Strip any existing system message at index 0 and prepend the new one.
	conversationMsgs := messages
	if len(conversationMsgs) > 0 && conversationMsgs[0].Role == "system" {
		conversationMsgs = conversationMsgs[1:]
	}

	assembled := make([]Message, 0, 1+len(conversationMsgs))
	assembled = append(assembled, systemMsg)
	assembled = append(assembled, conversationMsgs...)

	// Token budget enforcement (safety net using effective budget).
	if effectiveBudget > 0 {
		assembled = a.enforceTokenBudget(assembled, effectiveBudget, opts.SystemPrompt, memLimit, ragLimit, memories, ragResults)
	}

	return assembled, nil
}

// enforceTokenBudget trims messages to fit within the token budget.
//
// Truncation priority (drop first → last):
//  1. Older conversation messages (indices 1..end-2, excluding last user turn)
//  2. Reduce memory count and rebuild system message
//  3. Reduce RAG chunk count and rebuild system message
//  4. Collapse to only the system message and last user turn
//  5. Truncate the system message content
//  6. Truncate the last user message content
func (a *ContextAssembler) enforceTokenBudget(
	msgs []Message,
	maxTokens int,
	systemPrompt string,
	memLimit, ragLimit int,
	memories []MemoryResult,
	ragResults []RAGResult,
) []Message {
	if totalTokens(msgs) <= maxTokens {
		return msgs
	}

	// Build a mutable copy so we can splice messages out.
	result := make([]Message, len(msgs))
	copy(result, msgs)

	// Step 1: Drop oldest conversation messages one at a time.
	// We never drop result[0] (system message) or the last user message.
	for totalTokens(result) > maxTokens {
		// Find the last user message index (must be kept).
		lastUserInResult := -1
		for i := len(result) - 1; i >= 0; i-- {
			if result[i].Role == "user" {
				lastUserInResult = i
				break
			}
		}
		if lastUserInResult < 0 {
			break
		}

		// The drop zone is [1, lastUserInResult) — oldest first.
		dropStart := 1 // after system message (index 0)
		if dropStart >= lastUserInResult {
			// Nothing left to drop; move to next strategy.
			break
		}

		// Drop the oldest single message in the drop zone.
		result = append(result[:dropStart], result[dropStart+1:]...)
	}

	if totalTokens(result) <= maxTokens {
		return result
	}

	// Step 2: Reduce memory count.
	for len(memories) > 0 && totalTokens(result) > maxTokens {
		memories = memories[:len(memories)-1]
		_ = memLimit // used as initial cap only
		systemContent := buildSystemContent(systemPrompt, memories, ragResults)
		result[0] = Message{Role: "system", Content: systemContent}
	}

	if totalTokens(result) <= maxTokens {
		return result
	}

	// Step 3: Reduce RAG chunk count.
	for len(ragResults) > 0 && totalTokens(result) > maxTokens {
		ragResults = ragResults[:len(ragResults)-1]
		_ = ragLimit // used as initial cap only
		systemContent := buildSystemContent(systemPrompt, memories, ragResults)
		result[0] = Message{Role: "system", Content: systemContent}
	}

	if totalTokens(result) <= maxTokens {
		return result
	}

	// Step 4: Keep only the system message and the latest user message.
	lastUserInResult := -1
	for i := len(result) - 1; i >= 0; i-- {
		if result[i].Role == "user" {
			lastUserInResult = i
			break
		}
	}
	if lastUserInResult > 0 && len(result) > 2 {
		result = []Message{result[0], result[lastUserInResult]}
	}

	if totalTokens(result) <= maxTokens {
		return result
	}

	// Step 5: Truncate the system message to fit the remaining budget.
	if len(result) > 0 {
		remainingBudget := maxTokens
		for i := 1; i < len(result); i++ {
			remainingBudget -= estimateTokens(result[i])
		}
		result[0].Content = truncateContentToTokenBudget(result[0].Content, remainingBudget)
	}

	if totalTokens(result) <= maxTokens {
		return result
	}

	// Step 6: Truncate the last user message as a final safeguard.
	if len(result) > 1 {
		remainingBudget := maxTokens - estimateTokens(result[0])
		result[len(result)-1].Content = truncateContentToTokenBudget(result[len(result)-1].Content, remainingBudget)
	}

	return result
}
