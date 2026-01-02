# letterbox

IMAP-to-REST facade with webhook support. Expose email accounts via REST API with real-time notifications.

## Status

| Epic | Status |
|------|--------|
| 0. Project Foundation | Complete |
| 1. Account Management | Complete |
| 2. Email Reading | Complete |
| 3. Real-time Sync | Complete |
| 4. Webhook Engine | Complete |
| 5. Search | **Next** |
| 6. Production Readiness | Pending |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Coordinator                              │
│                                                                  │
│   AddAccount(id)                                                 │
│        │                                                         │
│        ▼                                                         │
│   Check IDLE capability                                          │
│        │                                                         │
│        ├── Yes ──► IdlePool (real-time, persistent connections) │
│        └── No ───► Poller (checks every 60s)                    │
│                                                                  │
│   ┌──────────────┐         ┌──────────────┐                     │
│   │   IdlePool   │         │    Poller    │                     │
│   │              │         │              │                     │
│   │  Account 1 ──┼────┐    │  Account 3 ──┼────┐                │
│   │  Account 2 ──┼────┤    │  Account 4 ──┼────┤                │
│   │    (IDLE)    │    │    │  (polling)   │    │                │
│   └──────────────┘    │    └──────────────┘    │                │
│                       │                        │                 │
│                       └───────────┬────────────┘                 │
│                                   ▼                              │
│                           handleEvent()                          │
│                                   │                              │
│                                   ▼                              │
│                       FetchUIDsAfter(lastUID)                   │
│                                   │                              │
│                                   ▼                              │
│                       Ingester.IngestEmail()                    │
│                          │              │                        │
│                          ▼              ▼                        │
│                    PostgreSQL     S3 (attachments)              │
│                                   │                              │
│                                   ▼                              │
│                         EventHandler(email)                      │
│                                   │                              │
│                                   ▼                              │
│                           Webhook Delivery                       │
└─────────────────────────────────────────────────────────────────┘
```

## Features

- REST API for reading emails across multiple IMAP accounts
- Real-time sync via IMAP IDLE (with polling fallback for non-IDLE servers)
- Webhooks on new email arrival with signature verification
- Full-text search via PostgreSQL (coming in Epic 5)
- Attachment storage on S3-compatible backends
- Encrypted credential storage

## API Endpoints

### Accounts
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/accounts` | Create account (validates IMAP creds) |
| GET | `/accounts` | List all accounts |
| GET | `/accounts/{id}` | Get account details |
| DELETE | `/accounts/{id}` | Delete account |

### Folders & Messages
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/accounts/{id}/folders` | List IMAP folders |
| GET | `/accounts/{id}/folders/{name}/messages` | List messages in folder |
| GET | `/accounts/{id}/messages/{uid}` | Get single message |

### Webhooks
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/webhooks` | Create webhook subscription |
| GET | `/webhooks` | List all webhooks |
| DELETE | `/webhooks/{id}` | Delete webhook |

### Health
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Liveness check |

## Webhook Signature Verification

All webhook payloads are signed using HMAC-SHA256. Verify signatures to ensure payloads are authentic and haven't been tampered with.

### Headers

| Header | Description |
|--------|-------------|
| `X-Letterbox-Signature` | HMAC-SHA256 signature (hex-encoded) |
| `X-Letterbox-Timestamp` | Unix timestamp when payload was signed |

### Signature Format

```
signature = HMAC-SHA256(timestamp + "." + payload, secret)
```

Where:
- `timestamp` is the value from `X-Letterbox-Timestamp` header
- `payload` is the raw request body (JSON)
- `secret` is your webhook secret (set when creating the webhook)

### Verification Steps

1. Extract `X-Letterbox-Timestamp` and `X-Letterbox-Signature` headers
2. Concatenate: `timestamp + "." + raw_body`
3. Compute HMAC-SHA256 using your webhook secret
4. Compare computed signature with `X-Letterbox-Signature` (constant-time comparison)
5. Optionally check timestamp to prevent replay attacks (reject if too old)

### Example: Go

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "net/http"
    "time"
)

func verifyWebhook(r *http.Request, body []byte, secret string) error {
    timestamp := r.Header.Get("X-Letterbox-Timestamp")
    signature := r.Header.Get("X-Letterbox-Signature")

    // Compute expected signature
    message := fmt.Sprintf("%s.%s", timestamp, body)
    h := hmac.New(sha256.New, []byte(secret))
    h.Write([]byte(message))
    expected := hex.EncodeToString(h.Sum(nil))

    // Constant-time comparison
    if !hmac.Equal([]byte(signature), []byte(expected)) {
        return fmt.Errorf("invalid signature")
    }

    // Optional: reject old timestamps (e.g., > 5 minutes)
    // ts, _ := strconv.ParseInt(timestamp, 10, 64)
    // if time.Now().Unix() - ts > 300 {
    //     return fmt.Errorf("timestamp too old")
    // }

    return nil
}
```

### Example: Python

```python
import hmac
import hashlib

def verify_webhook(timestamp: str, body: bytes, signature: str, secret: str) -> bool:
    message = f"{timestamp}.{body.decode()}"
    expected = hmac.new(
        secret.encode(),
        message.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(signature, expected)
```

### Example: Node.js

```javascript
const crypto = require('crypto');

function verifyWebhook(timestamp, body, signature, secret) {
    const message = `${timestamp}.${body}`;
    const expected = crypto
        .createHmac('sha256', secret)
        .update(message)
        .digest('hex');
    return crypto.timingSafeEqual(
        Buffer.from(signature),
        Buffer.from(expected)
    );
}
```

### Webhook Payload

```json
{
    "event": "email.received",
    "timestamp": "2024-01-15T10:30:00Z",
    "account_id": "550e8400-e29b-41d4-a716-446655440000",
    "email": {
        "id": "...",
        "uid": 12345,
        "message_id": "<abc@example.com>",
        "date": "2024-01-15T10:28:00Z",
        "from": {"name": "Alice", "email": "alice@example.com"},
        "to": [{"name": "Bob", "email": "bob@example.com"}],
        "cc": [],
        "subject": "Hello",
        "parsed": {"text": "...", "html": "..."},
        "attachments": [
            {
                "filename": "doc.pdf",
                "content_type": "application/pdf",
                "size": 12345,
                "url": "https://..."
            }
        ],
        "flags": ["\\Seen"],
        "folder": "INBOX"
    }
}
```

## Quick Start

```bash
# Start dependencies
docker-compose up -d

# Run migrations
make migrate-up

# Start server
make run
```

## Configuration

Copy `.envrc.example` to `.envrc` and configure:

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `S3_ENDPOINT` | S3-compatible storage endpoint |
| `S3_BUCKET` | Bucket for attachments |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | Storage credentials |
| `LETTERBOX_ENCRYPTION_KEY` | AES-256 key for credential encryption |
| `API_KEY` | API authentication key |

## Development

```bash
make help          # Show all commands
make build         # Build binary
make test          # Run tests
make migrate-up    # Apply migrations
make migrate-down  # Rollback migration
make sqlc          # Regenerate database code
```

## Tech Stack

- **Language**: Go
- **Database**: PostgreSQL (with full-text search)
- **Storage**: S3-compatible (Minio for local dev)
- **IMAP**: go-imap

## License

MIT
