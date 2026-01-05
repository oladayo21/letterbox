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
| POST | `/accounts` | Add IMAP/SMTP account |
| GET | `/accounts` | List accounts |
| GET | `/accounts/{id}` | Get account |
| DELETE | `/accounts/{id}` | Delete account |
| GET | `/accounts/{id}/folders` | List folders |
| GET | `/accounts/{id}/folders/{name}/messages` | List messages |
| GET | `/accounts/{id}/messages/{uid}?folder=INBOX` | Get message (folder defaults to INBOX) |
| POST | `/accounts/{id}/messages` | Send email (with reply/forward support) |
| POST | `/webhooks` | Create webhook |
| GET | `/webhooks` | List webhooks |
| DELETE | `/webhooks/{id}` | Delete webhook |
| GET | `/search?q=...&account_id=...` | Search emails |
| GET | `/health` | Liveness check |
| GET | `/ready` | Readiness check (DB, S3, sync status) |

## Sending Emails

Send emails via SMTP with optional reply/forward support.

### Basic Send

```json
POST /accounts/{id}/messages
{
  "to": [{"name": "John", "email": "john@example.com"}],
  "subject": "Hello",
  "text": "Plain text body",
  "html": "<p>HTML body</p>",
  "attachments": [{
    "filename": "doc.pdf",
    "content_type": "application/pdf",
    "data": "base64-encoded-content"
  }]
}
```

### Reply

```json
{
  "to": [{"email": "original-sender@example.com"}],
  "subject": "Re: Original Subject",
  "text": "My reply",
  "reply_to": 123,
  "folder": "INBOX"
}
```

Sets `In-Reply-To` and `References` headers for proper threading. Original message is quoted by default.

### Forward

```json
{
  "to": [{"email": "recipient@example.com"}],
  "subject": "Fwd: Original Subject",
  "text": "FYI",
  "forward": 456,
  "folder": "INBOX",
  "include_attachments": true
}
```

Options:
- `quote_original`: Include quoted original (default: true)
- `include_attachments`: Include original attachments (default: false)

## Webhooks

### Payload

Webhook payloads include the full email with attachments:
- Attachments < 1MB are base64-encoded inline (`data` field)
- Attachments >= 1MB use presigned S3 URLs (`url` field)

### Signature Verification

Payloads are signed with HMAC-SHA256. Headers:
- `X-Letterbox-Signature`: HMAC signature
- `X-Letterbox-Timestamp`: Unix timestamp

Signature formula:
```
signature = HMAC-SHA256(timestamp + "." + payload, secret)
```

Verification example (Go):
```go
func verifyWebhook(r *http.Request, secret string) ([]byte, error) {
    signature := r.Header.Get("X-Letterbox-Signature")
    timestamp := r.Header.Get("X-Letterbox-Timestamp")
    
    // Check timestamp is recent (within 5 minutes)
    ts, _ := strconv.ParseInt(timestamp, 10, 64)
    if time.Now().Unix()-ts > 300 {
        return nil, errors.New("timestamp too old")
    }
    
    body, _ := io.ReadAll(r.Body)
    
    // Compute expected signature
    message := fmt.Sprintf("%s.%s", timestamp, body)
    h := hmac.New(sha256.New, []byte(secret))
    h.Write([]byte(message))
    expected := hex.EncodeToString(h.Sum(nil))
    
    if !hmac.Equal([]byte(signature), []byte(expected)) {
        return nil, errors.New("invalid signature")
    }
    
    return body, nil
}
```

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
