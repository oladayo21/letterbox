# letterbox

IMAP-to-REST facade with webhook support.

## Architecture

```
IMAP Server ──► IDLE/Poller ──► Coordinator ──► Ingester ──► PostgreSQL
                                                   │
                                                   └──► S3 (attachments)
                                                   │
                                                   ▼
                                            Webhook Producer ──► Queue ──► Worker ──► Your Endpoint
```

## Quick Start

```bash
cp .env.example .env        # Configure environment
docker-compose up -d        # Start PostgreSQL + Minio
make migrate-up             # Run migrations
make run                    # Start server
```

## API

All endpoints require `X-API-Key` header.

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/accounts` | Add IMAP account |
| GET | `/accounts` | List accounts |
| GET | `/accounts/{id}` | Get account |
| DELETE | `/accounts/{id}` | Delete account |
| GET | `/accounts/{id}/folders` | List folders |
| GET | `/accounts/{id}/folders/{name}/messages` | List messages |
| GET | `/accounts/{id}/messages/{uid}` | Get message |
| POST | `/webhooks` | Create webhook |
| GET | `/webhooks` | List webhooks |
| DELETE | `/webhooks/{id}` | Delete webhook |
| GET | `/search?q=...&account_id=...` | Search emails |
| GET | `/health` | Liveness check |
| GET | `/ready` | Readiness check |

## Webhooks

Payloads are signed with HMAC-SHA256:

```
signature = HMAC-SHA256(timestamp + "." + payload, secret)
```

Headers: `X-Letterbox-Signature`, `X-Letterbox-Timestamp`

## Configuration

| Variable | Description |
|----------|-------------|
| `LETTERBOX_DATABASE_URL` | PostgreSQL connection string |
| `LETTERBOX_ENCRYPTION_KEY` | AES-256 key (64 hex chars) |
| `LETTERBOX_API_KEY` | API authentication key |
| `LETTERBOX_S3_ENDPOINT` | S3-compatible endpoint |
| `LETTERBOX_S3_BUCKET` | Bucket name |
| `LETTERBOX_S3_ACCESS_KEY` | S3 access key |
| `LETTERBOX_S3_SECRET_KEY` | S3 secret key |

Generate encryption key: `openssl rand -hex 32`

## License

MIT
