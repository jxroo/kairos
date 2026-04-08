// ---- Memory types (PascalCase — no json tags in Go) ----

export interface Memory {
  ID: string;
  Content: string;
  Summary: string;
  Importance: string;
  ConversationID: string;
  Source: string;
  ImportanceWeight: number;
  DecayScore: number;
  Tags: string[] | null;
  Entities: Entity[] | null;
  CreatedAt: string;
  UpdatedAt: string;
  AccessedAt: string;
  AccessCount: number;
}

export interface Entity {
  ID: string;
  Name: string;
  Type: string;
  MentionCount: number;
}

export interface CreateMemoryInput {
  Content: string;
  Context?: string;
  Importance?: string;
  ConversationID?: string;
  Source?: string;
  Tags?: string[];
}

export interface UpdateMemoryInput {
  Content?: string;
  Importance?: string;
  Tags?: string[];
}

export interface SearchResult {
  Memory: Memory;
  SimilarityScore: number;
  FinalScore: number;
}

// ---- Conversation types (snake_case — has json tags) ----

export interface Conversation {
  id: string;
  title: string;
  model: string;
  created_at: string;
  updated_at: string;
}

export interface ConversationMessage {
  id: string;
  conversation_id: string;
  role: "system" | "user" | "assistant" | "tool";
  content: string;
  tokens?: number;
  metadata?: string;
  created_at: string;
}

export interface ConversationDetail extends Conversation {
  messages: ConversationMessage[];
}

// ---- Inference types (snake_case — has json tags) ----

export interface ChatRequest {
  model: string;
  messages: ChatMessage[];
  temperature?: number;
  max_tokens?: number;
  stream?: boolean;
}

export interface ChatMessage {
  role: string;
  content: string;
  tool_calls?: ToolCallInfo[];
  tool_call_id?: string;
}

export interface ToolCallInfo {
  id: string;
  type: string;
  function: { name: string; arguments: string };
}

export interface OpenAIModelsResponse {
  object: string;
  data: ModelEntry[];
}

export interface ModelEntry {
  id: string;
  object: string;
  owned_by: string;
  context_length?: number;
  size_bytes?: number;
  capabilities?: string[];
}

// ---- RAG types (mixed) ----

// Document/Chunk: PascalCase (no json tags)
export interface Document {
  ID: string;
  Path: string;
  Filename: string;
  Extension: string;
  SizeBytes: number;
  FileHash: string;
  Status: "pending" | "indexing" | "indexed" | "error";
  ErrorMsg: string;
  CreatedAt: string;
  UpdatedAt: string;
  IndexedAt: string | null;
}

export interface Chunk {
  ID: string;
  DocumentID: string;
  ChunkIndex: number;
  Content: string;
  StartLine: number;
  EndLine: number;
  Metadata: string;
  CreatedAt: string;
}

export interface ChunkSearchResult {
  Chunk: Chunk;
  Document: Document;
  FinalScore: number;
}

// IndexStatus: snake_case (has json tags)
export interface IndexStatus {
  state: string;
  total_files: number;
  indexed_files: number;
  failed_files: number;
  percent: number;
}

// ---- Tool types (snake_case — has json tags) ----

export interface ToolDefinition {
  name: string;
  description: string;
  input_schema: Record<string, Param>;
  permissions: Permission[] | null;
  builtin: boolean;
}

export interface Param {
  type: string;
  description: string;
  required: boolean;
  default?: unknown;
  enum?: string[];
}

export interface Permission {
  resource: string;
  allow: boolean;
  paths?: string[];
}

export interface ToolResult {
  content: string;
  is_error: boolean;
}

export interface AuditEntry {
  id: string;
  tool_name: string;
  arguments: string;
  result: string;
  is_error: boolean;
  duration_ms: number;
  caller: string;
  created_at: string;
}

// ---- Health ----

export interface HealthResponse {
  status: string;
  version: string;
  uptime: string;
}

export interface ConfigResponse {
  path: string;
  content: string;
  writable: boolean;
}

export interface UpdateConfigInput {
  content: string;
}

export interface UpdateConfigResponse {
  path: string;
  reload_required: boolean;
}
