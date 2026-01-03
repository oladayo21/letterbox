# letterbox

IMAP-to-REST facade service with webhook support.

## Overview

Exposes IMAP accounts over REST API with real-time webhook notifications. Eliminates rebuilding email integration for each app.

## Core Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Stack | Go | Efficient for long-running IMAP connections |
| Database | PostgreSQL | Robust, JSON support, FTS via tsvector |
| Storage | S3-compatible | Attachments to Minio/R2/etc |
| Auth | API keys | Simple, sufficient for single-user |
| Deployment | Self-hosted single-user | Start simple, evolve later |

## Architecture

```
┌─────────────┐      ┌─────────────────────────────────────────┐
│ Client Apps │◄────►│              letterbox                  │
└─────────────┘      │  ┌─────────┐  ┌─────────┐  ┌─────────┐  │
                     │  │ REST API│  │ Webhook │  │  Sync   │  │
                     │  │ Server  │  │ Engine  │  │ Worker  │  │
                     │  └────┬────┘  └────┬────┘  └────┬────┘  │
                     │       │            │            │       │
                     │  ┌────┴────────────┴────────────┴────┐  │
                     │  │           PostgreSQL              │  │
                     │  │  (accounts, emails, queue, FTS)   │  │
                     │  └───────────────────────────────────┘  │
                     │       │                                 │
                     │  ┌────┴────────────┐                    │
                     │  │  S3 (attachments)│                   │
                     │  └─────────────────┘                    │
                     └─────────────────────────────────────────┘
                                    │
                           ┌────────┴────────┐
                           │   IMAP Servers  │
                           │ (Gmail, Outlook,│
                           │   Fastmail...)  │
                           └─────────────────┘
```

## API Design

Resource-centric REST:

```
GET    /accounts                     # List configured accounts
POST   /accounts                     # Add account (IMAP + SMTP config)
GET    /accounts/{id}                # Get account details
DELETE /accounts/{id}                # Remove account

GET    /accounts/{id}/folders        # List folders
GET    /accounts/{id}/folders/{name}/messages
GET    /accounts/{id}/messages/{uid} # Get single message

POST   /accounts/{id}/messages       # Send email (SMTP)
PATCH  /accounts/{id}/messages/{uid} # Move, mark read/unread, flag
DELETE /accounts/{id}/messages/{uid} # Delete (marks deleted upstream)

GET    /webhooks                     # List subscriptions
POST   /webhooks                     # Create subscription
DELETE /webhooks/{id}                # Remove subscription

GET    /search?q=...&account=...     # Full-text search
```

## Email Response Format

Always returns both parsed and raw:

```json
{
  "uid": 12345,
  "message_id": "<abc@example.com>",
  "date": "2025-01-15T10:30:00Z",
  "from": {"name": "John", "email": "john@example.com"},
  "to": [{"name": "You", "email": "you@domain.com"}],
  "subject": "Hello",
  "parsed": {
    "text": "Plain text body...",
    "html": "<p>HTML body...</p>"
  },
  "raw": "raw MIME content...",
  "attachments": [
    {
      "filename": "doc.pdf",
      "content_type": "application/pdf",
      "size": 524288,
      "url": "https://s3.../attachments/abc123/doc.pdf"
    }
  ],
  "flags": ["\\Seen"],
  "folder": "INBOX"
}
```

## Webhook System

### Subscription

```json
POST /webhooks
{
  "account_id": "uuid",
  "url": "https://myapp.com/hooks/email",
  "secret": "hmac-secret-for-verification"
}
```

All emails from the account → endpoint. (Filtering can be added later.)

### Delivery

- **Trigger**: Hybrid (IMAP IDLE where supported, polling fallback)
- **Payload**: Full parsed email (same format as API response)
- **Attachments**: Inline if <1MB, S3 URL if larger
- **Retry**: Exponential backoff, persisted queue
- **Signature**: HMAC-SHA256 in `X-Letterbox-Signature` header

### Payload

```json
{
  "event": "email.received",
  "timestamp": "2025-01-15T10:30:00Z",
  "account_id": "uuid",
  "email": { /* full email object */ }
}
```

## Data Model

### accounts
```sql
id              UUID PRIMARY KEY
name            TEXT
imap_host       TEXT
imap_port       INT
imap_user       TEXT
imap_password   TEXT (encrypted)
smtp_host       TEXT
smtp_port       INT
smtp_user       TEXT
smtp_password   TEXT (encrypted)
created_at      TIMESTAMP
```

### emails
```sql
id              UUID PRIMARY KEY
account_id      UUID REFERENCES accounts
uid             INT
message_id      TEXT
folder          TEXT
subject         TEXT
from_email      TEXT
from_name       TEXT
date            TIMESTAMP
parsed_text     TEXT
parsed_html     TEXT
raw             TEXT
flags           TEXT[]
deleted_upstream BOOLEAN DEFAULT false
search_vector   TSVECTOR
created_at      TIMESTAMP
```

### attachments
```sql
id              UUID PRIMARY KEY
email_id        UUID REFERENCES emails
filename        TEXT
content_type    TEXT
size            INT
s3_key          TEXT
```

### webhooks
```sql
id              UUID PRIMARY KEY
account_id      UUID REFERENCES accounts
url             TEXT
secret          TEXT (encrypted)
created_at      TIMESTAMP
```

### webhook_queue
```sql
id              UUID PRIMARY KEY
webhook_id      UUID REFERENCES webhooks
email_id        UUID REFERENCES emails
payload         JSONB
attempts        INT DEFAULT 0
next_attempt    TIMESTAMP
status          TEXT (pending, delivered, failed)
created_at      TIMESTAMP
```

## Connection Management

- **IDLE pool**: Persistent connections for accounts with IDLE support (real-time)
- **Polling worker**: Check non-IDLE accounts every 60s
- **On-demand**: Open connection for API operations, close after

## Credential Security

- AES-256 encryption for IMAP/SMTP passwords
- Encryption key from `LETTERBOX_ENCRYPTION_KEY` env var
- Never log or expose credentials in API responses

## Sync Strategy

- **Initial**: Start empty (no historical sync)
- **Backfill**: Separate background script for history import
- **Source of truth**: IMAP server
- **Deletion**: Archive mode (flag `deleted_upstream`, never delete locally)
- **Sync**: On new email (via IDLE/poll), parse immediately, store parsed result

## Search

Postgres full-text search:

```sql
CREATE INDEX emails_search_idx ON emails USING GIN(search_vector);

-- Search query
SELECT * FROM emails
WHERE account_id = $1
  AND search_vector @@ plainto_tsquery('english', $2)
ORDER BY date DESC;
```

## MVP Scope

Phase 1 (MVP) - COMPLETE:
- [x] Account management (IMAP config only)
- [x] Read emails via REST
- [x] List folders
- [x] Webhook subscriptions
- [x] Real-time new email notifications
- [x] Full-text search
- [x] Attachment storage

Phase 2:
- [ ] SMTP send
- [ ] Move/delete/flag operations
- [ ] Webhook filters (folder, sender, subject patterns)
- [ ] History backfill tool

Phase 3:
- [ ] Multi-user / API key management
- [ ] Rate limiting
- [ ] OAuth provider support

## Tech Stack

- **Language**: Go
- **IMAP**: go-imap
- **HTTP**: Chi or Fiber
- **Database**: PostgreSQL + sqlc
- **Queue**: PostgreSQL-based (no Redis needed)
- **Storage**: S3-compatible via aws-sdk-go-v2
- **Config**: Viper or envconfig

## Project Structure

```
letterbox/
├── cmd/
│   └── letterbox/
│       └── main.go
├── internal/
│   ├── api/           # REST handlers
│   ├── imap/          # IMAP client, connection pool
│   ├── smtp/          # Send email
│   ├── webhook/       # Delivery engine
│   ├── sync/          # IDLE + polling workers
│   ├── storage/       # S3 adapter
│   ├── parser/        # Email parsing
│   ├── crypto/        # Encryption helpers
│   └── db/            # sqlc generated code
├── migrations/        # SQL migrations
├── Makefile
├── go.mod
└── SPEC.md
```
