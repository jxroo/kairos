#!/bin/bash
set -euo pipefail

BASE_URL="${KAIROS_URL:-http://localhost:7777}"
VERBOSE=false

for arg in "$@"; do
  case "$arg" in
    --verbose|-v) VERBOSE=true ;;
    *) echo "Unknown option: $arg"; exit 1 ;;
  esac
done

post() {
  local endpoint="$1"
  local data="$2"
  if $VERBOSE; then
    curl -sf -X POST "$BASE_URL$endpoint" \
      -H "Content-Type: application/json" \
      -d "$data"
    echo ""
  else
    curl -sf -X POST "$BASE_URL$endpoint" \
      -H "Content-Type: application/json" \
      -d "$data" > /dev/null
  fi
}

echo "Seeding Kairos with demo data..."
echo "Target: $BASE_URL"
echo ""

# Check if Kairos is running
curl -sf "$BASE_URL/health" > /dev/null || {
  echo "Error: Kairos is not running at $BASE_URL"
  echo "Start it with: kairos start"
  exit 1
}

# --- Memories ---
echo "Creating memories..."

echo "  [1/10] Project deadline reminder"
post "/memories" '{
  "content": "Atlas v2.0 release deadline is March 28th. All feature branches must be merged by March 25th to allow three days for integration testing and staging deployment. QA sign-off required from both platform and data teams before production push.",
  "importance": "high"
}'

echo "  [2/10] Team standup notes"
post "/memories" '{
  "content": "Standup notes March 15: Elena finished the Kafka consumer refactor, PR ready for review. Marcus is blocked on the IAM permissions for the new S3 buckets — waiting on DevOps ticket INFRA-2847. Sofia started writing integration tests for the notification pipeline. Overall sprint velocity looks on track.",
  "importance": "normal"
}'

echo "  [3/10] Database migration decision"
post "/memories" '{
  "content": "Technical decision: we are migrating from PostgreSQL 14 to PostgreSQL 16 for the Atlas project. Key reasons: improved JSONB query performance (40% faster in our benchmarks), native logical replication enhancements, and better partitioning support for the events table which is approaching 800M rows. Migration window scheduled for April 5-6 weekend.",
  "importance": "high"
}'

echo "  [4/10] Personal reminder"
post "/memories" '{
  "content": "Dentist appointment on Thursday at 2:30 PM with Dr. Kowalski. Need to leave the office by 2:00 to make it on time. Remember to bring the referral letter from Dr. Nowak.",
  "importance": "low"
}'

echo "  [5/10] API design discussion"
post "/memories" '{
  "content": "API design review for Notification Service v2: agreed on REST over gRPC for external consumers, gRPC for internal service-to-service calls. Rate limiting will be 1000 req/min per API key for the free tier, 10000 for premium. Webhook delivery needs at-least-once semantics with exponential backoff. Max payload size stays at 64KB.",
  "importance": "normal"
}'

echo "  [6/10] Security review findings"
post "/memories" '{
  "content": "Security review completed for the Atlas ingestion pipeline. Found three issues: (1) CRITICAL — raw SQL interpolation in the legacy batch importer, must fix before release. (2) MEDIUM — JWT tokens not rotated on password change, add to sprint backlog. (3) LOW — verbose error messages in staging expose internal paths, cosmetic fix. Tracking in JIRA epic SEC-142.",
  "importance": "high"
}'

echo "  [7/10] Book recommendation"
post "/memories" '{
  "content": "Sofia recommended \"Designing Data-Intensive Applications\" by Martin Kleppmann. She said chapters 5-7 on replication and partitioning are especially relevant to the problems we are solving with the Atlas event store. Available in the office library, second shelf.",
  "importance": "low"
}'

echo "  [8/10] Performance optimization results"
post "/memories" '{
  "content": "Performance optimization round 2 results: after switching the event serialization from JSON to MessagePack and adding connection pooling (max 50 conns), ingestion throughput went from 12k events/sec to 31k events/sec. P99 latency dropped from 45ms to 18ms. Memory usage reduced by 22%. Benchmarks recorded in docs/benchmarks/2026-03-14.md.",
  "importance": "normal"
}'

echo "  [9/10] Deployment checklist"
post "/memories" '{
  "content": "Production deployment checklist for Atlas v2.0: (1) Run database migrations with --dry-run first. (2) Scale down consumer pods to zero. (3) Deploy new schema via Flyway. (4) Update Kafka topic configs (retention 7d -> 14d). (5) Deploy application pods with canary at 10% traffic. (6) Monitor error rates for 30 minutes. (7) Full rollout if error rate < 0.1%. (8) Scale consumers back up. (9) Verify dashboard metrics. (10) Send all-clear to #atlas-releases.",
  "importance": "high"
}'

echo "  [10/10] Coffee chat notes"
post "/memories" '{
  "content": "Had coffee with Tomek, the new backend engineer who joined last Monday. He previously worked at Allegro on their search infrastructure team. Interested in distributed systems and has experience with Elasticsearch and ClickHouse. Seems like a great fit for the analytics pipeline work planned for Q2. Offered to help him with onboarding next week.",
  "importance": "low"
}'

echo ""
echo "Memories created: 10"

# --- Conversations ---
echo ""
echo "Creating conversations..."
echo "  (Conversations require a running LLM provider via Ollama or llama.cpp)"

# Attempt a simple conversation; skip gracefully if no provider is available
CONV_OK=true
curl -sf -X POST "$BASE_URL/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "",
    "messages": [
      {"role": "user", "content": "Summarize the key risks for the Atlas v2.0 release."}
    ]
  }' > /dev/null 2>&1 || CONV_OK=false

if $CONV_OK; then
  echo "  [1/2] Created conversation: Atlas release risks"

  curl -sf -X POST "$BASE_URL/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
      "model": "",
      "messages": [
        {"role": "user", "content": "What is the current status of the database migration to PostgreSQL 16? Are there any blockers?"}
      ]
    }' > /dev/null 2>&1 || true
  echo "  [2/2] Created conversation: Database migration status"
else
  echo "  Skipped — no LLM provider available. Start Ollama to enable conversations."
fi

# --- RAG Indexing ---
echo ""
echo "Triggering RAG indexing..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DOCS_DIR="$SCRIPT_DIR/demo-docs"

if [ -d "$DEMO_DOCS_DIR" ]; then
  echo "  Demo documents found in $DEMO_DOCS_DIR"
  echo "  Ensure this path is listed in your config.toml under [rag] watch_paths"
  echo "  Triggering index rebuild..."
  curl -sf -X POST "$BASE_URL/index/rebuild" > /dev/null 2>&1 || {
    echo "  Note: Index rebuild endpoint not available or watch_paths not configured."
    echo "  Add the following to ~/.kairos/config.toml:"
    echo "    [rag]"
    echo "    watch_paths = [\"$DEMO_DOCS_DIR\"]"
  }
else
  echo "  Warning: demo-docs/ directory not found at $DEMO_DOCS_DIR"
fi

echo ""
echo "========================================="
echo " Demo data seeded successfully!"
echo "========================================="
echo ""
echo " Memories:       10 created"
$CONV_OK && echo " Conversations:  2 created" || echo " Conversations:  skipped (no LLM)"
echo ""
echo " Open the dashboard:"
echo "   $BASE_URL/dashboard/"
echo ""
echo " Try a semantic search:"
echo "   curl \"$BASE_URL/memories/search?query=database+migration\""
echo ""
