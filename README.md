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
| 5. Search | Complete |
| 6. Production Readiness | Complete |

## Architecture

```mermaid
graph TB
    subgraph Clients
        APP[Client Apps]
    end

    subgraph letterbox[Letterbox Service]
        subgraph API[API Layer]
            REST[REST API<br/>Chi Router]
            AUTH[API Key Auth]
        end

        subgraph Handlers
            AH[Account Handler]
            FH[Folder Handler]
            MH[Message Handler]
            WH[Webhook Handler]
            SH[Search Handler]
        end

        subgraph Sync[Sync System]
            COORD[Coordinator]
            IDLE[IDLE Pool<br/>Real-time]
            POLL[Poller<br/>60s interval]
        end

        subgraph Webhook[Webhook System]
            PROD[Producer<br/>Queue emails]
            WORK[Worker<br/>Deliver + Retry]
        end

        subgraph Core[Core Services]
            ING[Ingester<br/>Fetch → Parse → Store]
            PARSE[Parser<br/>RFC822 → Structured]
            IMAPC[IMAP Client]
        end

        subgraph Data[Data Layer]
            REPO[Repositories]
            CRYPT[Crypto<br/>AES-256]
            S3C[S3 Storage]
        end
    end

    subgraph Storage
        PG[(PostgreSQL<br/>emails, webhooks<br/>queue, FTS)]
        MINIO[(S3 Compatible<br/>Attachments)]
    end

    subgraph External
        IMAP[IMAP Servers<br/>Gmail, Outlook, etc.]
        WEBHOOK_EP[Webhook Endpoints]
    end

    APP -->|HTTP + API Key| AUTH
    AUTH --> REST
    REST --> AH & FH & MH & WH & SH
    AH & FH & MH & SH --> REPO
    MH -->|on-demand fetch| ING
    WH --> REPO

    COORD --> IDLE & POLL
    IDLE & POLL <-->|IMAP| IMAP
    IDLE & POLL -->|new email| COORD
    COORD --> ING
    ING --> IMAPC
    IMAPC --> IMAP
    ING --> PARSE
    ING --> REPO
    ING --> S3C

    COORD -->|on new email| PROD
    PROD --> PG
    WORK -->|poll queue| PG
    WORK -->|POST + HMAC| WEBHOOK_EP

    REPO --> PG
    REPO --> CRYPT
    S3C --> MINIO
```

### Sequence: New Email Arrives

```mermaid
sequenceDiagram
    participant IMAP as IMAP Server
    participant IDLE as IDLE Pool
    participant COORD as Coordinator
    participant ING as Ingester
    participant DB as PostgreSQL
    participant S3 as S3 Storage
    participant PROD as Webhook Producer
    participant WORK as Webhook Worker
    participant EP as Webhook Endpoint

    IMAP->>IDLE: EXISTS (new message)
    IDLE->>COORD: IdleEvent{NewMessage}
    COORD->>IMAP: Fetch UIDs after last known
    IMAP-->>COORD: [UID 123]
    
    COORD->>ING: IngestEmail(account, folder, uid)
    ING->>IMAP: Fetch raw RFC822
    IMAP-->>ING: raw bytes
    ING->>ING: Parse email
    ING->>S3: Upload attachments
    ING->>DB: Store email + attachments
    ING-->>COORD: *domain.Email

    COORD->>PROD: QueueForEmail(email)
    PROD->>DB: Insert queue items

    loop Worker polling (every 5s)
        WORK->>DB: GetPendingItems
        DB-->>WORK: [item]
        WORK->>EP: POST with HMAC signature
        alt Success
            EP-->>WORK: 200 OK
            WORK->>DB: Mark delivered
        else Failure
            EP-->>WORK: Error
            WORK->>DB: Schedule retry with backoff
        end
    end
```

### Package Structure

```mermaid
graph LR
    subgraph cmd
        MAIN[main.go]
    end

    subgraph internal
        API[api]
        SYNC[sync]
        WEBHOOK[webhook]
        INGEST[ingest]
        IMAPC[imap]
        PARSER[parser]
        REPO[repository]
        STORAGE[storage]
        DB[db]
        CRYPTO[crypto]
        DOMAIN[domain]
    end

    MAIN --> API & SYNC & WEBHOOK & INGEST & REPO & STORAGE

    API --> REPO & INGEST & STORAGE & IMAPC
    SYNC --> IMAPC & INGEST & REPO
    WEBHOOK --> REPO & STORAGE & DB
    INGEST --> IMAPC & PARSER & REPO & STORAGE
    REPO --> DB & CRYPTO & DOMAIN
    PARSER --> DOMAIN
```

| Package | Purpose |
|---------|---------|
| `cmd/letterbox` | Entry point, wires dependencies |
| `internal/api` | REST handlers |
| `internal/sync` | Coordinator + IDLE pool + Poller |
| `internal/webhook` | Producer (queue) + Worker (deliver) |
| `internal/ingest` | Fetch → Parse → Store pipeline |
| `internal/imap` | IMAP client helpers |
| `internal/parser` | RFC822 email parsing |
| `internal/repository` | Data access with encryption |
| `internal/storage` | S3 adapter |
| `internal/db` | sqlc-generated queries |
| `internal/crypto` | AES-256 encryption |
| `internal/domain` | Core types |

## Features

- REST API for reading emails across multiple IMAP accounts
- Real-time sync via IMAP IDLE (with polling fallback for non-IDLE servers)
- Webhooks on new email arrival with signature verification
- Full-text search via PostgreSQL
- Attachment storage on S3-compatible backends
- Encrypted credential storage
- Health and readiness endpoints
- Graceful shutdown handling
- Docker support

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

### Search
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/search?q=...&account_id=...` | Full-text search |

### Health
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Liveness check (always 200) |
| GET | `/ready` | Readiness check (DB + S3) |

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

## Docker

### Build Image

```bash
make docker-build
# or with custom tag
make docker-build DOCKER_TAG=v1.0.0
```

### Run Container

```bash
# With .env file
docker run --rm -p 8080:8080 --env-file .env letterbox:latest

# Or with explicit env vars
docker run --rm -p 8080:8080 \
  -e LETTERBOX_DATABASE_URL="postgres://..." \
  -e LETTERBOX_ENCRYPTION_KEY="..." \
  -e LETTERBOX_API_KEY="..." \
  -e LETTERBOX_S3_ENDPOINT="..." \
  -e LETTERBOX_S3_BUCKET="letterbox" \
  -e LETTERBOX_S3_ACCESS_KEY="..." \
  -e LETTERBOX_S3_SECRET_KEY="..." \
  letterbox:latest
```

### Run Migrations

Migrations are included in the Docker image at `/app/migrations`. Run them using golang-migrate:

```bash
docker run --rm \
  -v $(pwd)/migrations:/migrations \
  migrate/migrate \
  -path=/migrations \
  -database="postgres://user:pass@host:5432/letterbox?sslmode=disable" \
  up
```

## Configuration

Copy `.env.example` to `.env` and configure:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LETTERBOX_DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `LETTERBOX_ENCRYPTION_KEY` | Yes | - | AES-256 key (64 hex chars) for credential encryption |
| `LETTERBOX_API_KEY` | Yes | - | API authentication key |
| `LETTERBOX_S3_ENDPOINT` | Yes | - | S3-compatible storage endpoint |
| `LETTERBOX_S3_BUCKET` | Yes | - | Bucket for attachments |
| `LETTERBOX_S3_ACCESS_KEY` | Yes | - | S3 access key |
| `LETTERBOX_S3_SECRET_KEY` | Yes | - | S3 secret key |
| `LETTERBOX_S3_REGION` | No | `auto` | S3 region |
| `LETTERBOX_PORT` | No | `8080` | HTTP server port |
| `LETTERBOX_LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `LETTERBOX_LOG_FORMAT` | No | `json` | Log format (json, text) |

Generate encryption key: `openssl rand -hex 32`

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
