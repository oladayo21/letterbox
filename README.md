# letterbox

IMAP-to-REST facade with webhook support. Expose email accounts via REST API with real-time notifications.

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
- Real-time webhooks on new email (IMAP IDLE + polling fallback)
- Full-text search via PostgreSQL
- Attachment storage on S3-compatible backends
- Encrypted credential storage

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
