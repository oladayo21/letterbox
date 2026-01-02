# letterbox

IMAP-to-REST facade with webhook support. Expose email accounts via REST API with real-time notifications.

## Status

| Epic | Status |
|------|--------|
| 0. Project Foundation | Complete |
| 1. Account Management | Complete |
| 2. Email Reading | Complete |
| 3. Real-time Sync | Complete |
| 4. Webhook Engine | **Next** |
| 5. Search | Pending |
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
- Webhooks on new email arrival (coming in Epic 4)
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

### Health
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Liveness check |

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
