# Notification Service v2 — API Design Document

**Author:** Sofia Kowalska
**Status:** Draft
**Last Updated:** March 14, 2026
**Reviewers:** Elena Vasquez, Marcus Brandt

---

## Overview

Notification Service v2 replaces the legacy notification system with a unified API
for managing notification channels, templates, and delivery. Key improvements over v1
include webhook support with guaranteed at-least-once delivery, template rendering with
variable substitution, and per-consumer rate limiting.

The service handles email, SMS, push notifications, and webhooks through a single
consistent interface. All delivery is asynchronous via Kafka, with status tracking
available through polling or webhook callbacks.

## Base URL

```
Production:  https://api.internal.atlas.dev/notifications/v2
Staging:     https://api.staging.atlas.dev/notifications/v2
```

## Authentication

All requests require a valid API key passed in the `X-API-Key` header.
Keys are provisioned through the IAM service and scoped per team.

## Endpoints

### POST /notifications

Send a new notification.

**Request:**
```json
{
  "channel": "email",
  "recipient": "user@example.com",
  "template_id": "welcome-email-v3",
  "variables": {
    "user_name": "Elena",
    "activation_url": "https://app.atlas.dev/activate?token=abc123"
  },
  "priority": "normal",
  "idempotency_key": "signup-12345"
}
```

**Response (202 Accepted):**
```json
{
  "id": "ntf_8f3a2b1c",
  "status": "queued",
  "channel": "email",
  "created_at": "2026-03-14T10:30:00Z",
  "estimated_delivery": "2026-03-14T10:30:05Z"
}
```

Supported channels: `email`, `sms`, `push`, `webhook`.

### GET /notifications/{id}

Retrieve the status and delivery details of a notification.

**Response (200 OK):**
```json
{
  "id": "ntf_8f3a2b1c",
  "status": "delivered",
  "channel": "email",
  "recipient": "user@example.com",
  "created_at": "2026-03-14T10:30:00Z",
  "delivered_at": "2026-03-14T10:30:03Z",
  "attempts": 1
}
```

Possible statuses: `queued`, `sending`, `delivered`, `failed`, `bounced`.

### POST /webhooks

Register a webhook endpoint for delivery callbacks.

**Request:**
```json
{
  "url": "https://myservice.atlas.dev/callbacks/notifications",
  "events": ["delivered", "failed", "bounced"],
  "secret": "whsec_a1b2c3d4e5"
}
```

**Response (201 Created):**
```json
{
  "id": "whk_9d4e5f6a",
  "url": "https://myservice.atlas.dev/callbacks/notifications",
  "events": ["delivered", "failed", "bounced"],
  "active": true,
  "created_at": "2026-03-14T11:00:00Z"
}
```

Webhook payloads are signed with HMAC-SHA256 using the provided secret.
Failed deliveries are retried with exponential backoff: 1s, 5s, 30s, 5m, 30m (max 5 attempts).

### GET /templates

List available notification templates with filtering.

**Query Parameters:**
- `channel` — filter by channel (optional)
- `search` — full-text search on template name and content (optional)
- `page`, `per_page` — pagination (default: page=1, per_page=20)

**Response (200 OK):**
```json
{
  "templates": [
    {
      "id": "welcome-email-v3",
      "name": "Welcome Email",
      "channel": "email",
      "variables": ["user_name", "activation_url"],
      "updated_at": "2026-02-28T14:00:00Z"
    }
  ],
  "total": 42,
  "page": 1,
  "per_page": 20
}
```

## Rate Limiting

Rate limits are enforced per API key using a sliding window algorithm.

| Tier | Limit | Burst |
|------|-------|-------|
| Free | 1,000 requests/min | 50 |
| Standard | 5,000 requests/min | 200 |
| Premium | 10,000 requests/min | 500 |

Rate limit headers are included in every response:
- `X-RateLimit-Limit` — maximum requests per window
- `X-RateLimit-Remaining` — requests remaining in current window
- `X-RateLimit-Reset` — Unix timestamp when the window resets

Exceeding the limit returns `429 Too Many Requests` with a `Retry-After` header.

## Migration from v1

### Breaking Changes
1. The `send` field is renamed to `channel` in all request bodies.
2. Synchronous delivery mode is removed. All notifications are now asynchronous.
3. The `/notifications/batch` endpoint is replaced by accepting arrays in `POST /notifications`.
4. Error response format follows RFC 7807 (Problem Details).

### Migration Timeline
- **March 21:** v2 available in staging for integration testing.
- **March 28:** v2 deployed to production alongside v1.
- **April 30:** v1 deprecated, returns warning headers.
- **June 30:** v1 endpoints removed.

### Compatibility Layer
During the transition period, requests to v1 endpoints are proxied to v2 with automatic
field mapping. A `X-API-Version: v1-compat` header is added to proxied responses so
consumers can detect and update their integrations.
