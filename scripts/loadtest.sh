#!/bin/bash
set -euo pipefail

# Kairos HTTP Load Test
# Prerequisites: hey (go install github.com/rakyll/hey@latest)
# Usage: ./scripts/loadtest.sh [base_url] [duration] [concurrency]

BASE_URL="${1:-http://localhost:7777}"
DURATION="${2:-10s}"
CONCURRENCY="${3:-10}"
REQUESTS=200

# Check for hey.
if ! command -v hey &>/dev/null; then
    echo "Error: 'hey' is not installed."
    echo "Install with: go install github.com/rakyll/hey@latest"
    exit 1
fi

echo "=== Kairos Load Test ==="
echo "Base URL:    $BASE_URL"
echo "Duration:    $DURATION"
echo "Concurrency: $CONCURRENCY"
echo ""

# Seed test data if --seed flag is passed.
if [[ "${4:-}" == "--seed" ]]; then
    echo "--- Seeding test data ---"
    for i in $(seq 1 10); do
        curl -s -X POST "$BASE_URL/memories" \
            -H "Content-Type: application/json" \
            -d "{\"content\":\"Load test memory entry $i about topic $(( i % 5 ))\",\"importance\":\"normal\"}" \
            >/dev/null
    done
    echo "Seeded 10 test memories."
    echo ""
fi

echo "--- Scenario 1: GET /health (baseline) ---"
hey -n "$REQUESTS" -c "$CONCURRENCY" -z "$DURATION" "$BASE_URL/health"
echo ""

echo "--- Scenario 2: GET /v1/models ---"
hey -n "$REQUESTS" -c "$CONCURRENCY" -z "$DURATION" "$BASE_URL/v1/models" 2>/dev/null || echo "(skipped — inference may not be available)"
echo ""

echo "--- Scenario 3: GET /memories/search ---"
hey -n "$REQUESTS" -c "$CONCURRENCY" -z "$DURATION" "$BASE_URL/memories/search?query=test&limit=5"
echo ""

echo "--- Scenario 4: POST /memories (write) ---"
hey -n "$REQUESTS" -c "$CONCURRENCY" -z "$DURATION" \
    -m POST \
    -H "Content-Type: application/json" \
    -d '{"content":"load test memory entry","importance":"normal"}' \
    "$BASE_URL/memories"
echo ""

echo "=== Load test complete ==="
