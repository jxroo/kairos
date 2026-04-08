# Scripts

## loadtest.sh

HTTP load test for Kairos API endpoints using [hey](https://github.com/rakyll/hey).

### Prerequisites

```bash
go install github.com/rakyll/hey@latest
```

### Usage

```bash
# Default: localhost:7777, 10s duration, 10 concurrent
./scripts/loadtest.sh

# Custom target
./scripts/loadtest.sh http://localhost:7777 30s 20

# Seed test data first
./scripts/loadtest.sh http://localhost:7777 10s 10 --seed
```

### Scenarios

1. `GET /health` — baseline HTTP roundtrip
2. `GET /v1/models` — model listing (requires inference provider)
3. `GET /memories/search` — memory search
4. `POST /memories` — memory write throughput
