# Kairos Security Audit — Phase 5

Date: 2026-03-15
Scope: Tool runtime, HTTP API, permission system

## Finding 1: SSRF in Sandbox HTTP Client (High)

**Location:** `internal/tools/sandbox.go` — `doHTTPRequest` used `http.DefaultClient`

**Risk:** Tool scripts with network permission could make requests to internal services (localhost, private IPs), potentially accessing metadata endpoints or internal APIs.

**Fix:** Created `safeSandboxClient()` returning an `*http.Client` with:
- 30s timeout
- Max 5 redirects via `CheckRedirect`
- Custom `Transport` with `DialContext` that resolves target IPs and rejects private ranges: 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, ::1/128, fc00::/7

**Test:** `TestSandbox_SSRFBlocked` verifies requests to localhost, 10.x, 172.16.x, 192.168.x, and ::1 are rejected.

## Finding 2: Symlink Traversal in Path Permissions (Medium)

**Location:** `internal/tools/permissions.go` — `pathAllowed` used `filepath.Abs` + `strings.HasPrefix`

**Risk:** A symlink inside an allowed directory pointing outside it would bypass the path check, allowing tools to read/write files outside the permitted scope.

**Fix:** Added `filepath.EvalSymlinks()` after `filepath.Abs()` for both the target path and each allowed path. If symlink resolution fails (broken symlink), access is denied.

**Test:** `TestSymlinkTraversal` creates a symlink `/tmp/allowed/escape -> /etc/` and verifies `pathAllowed` denies `escape/passwd`.

## Finding 3: JSON Injection in Error Response (Medium)

**Location:** `internal/server/tools_handlers.go` — string concatenation `{"error":"` + err.Error() + `"}`

**Risk:** Error messages containing quotes or special characters could break JSON structure, potentially leading to response splitting or client-side parsing issues.

**Fix:** Replaced string concatenation with `json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})`.

**Test:** `TestHandleExecuteToolErrorIsValidJSON` verifies error responses parse as valid JSON.

## Finding 4: No Request Body Size Limit (Low)

**Location:** `internal/server/server.go` — routes had no body limit middleware

**Risk:** Attackers could send arbitrarily large request bodies, causing memory exhaustion.

**Fix:** Added `maxBodySize(1 << 20)` middleware (1MB limit) applied to all POST/PUT routes via `http.MaxBytesReader`.

**Test:** `TestBodySizeLimitRejectsLargePayload` sends a 2MB POST body and verifies rejection.
