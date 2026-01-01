# letterbox - Development Stories

Stories ordered for sequential implementation. Each story is independently deployable where possible.

---

## Epic 0: Project Foundation
*Setup the project skeleton and development environment*

### Story 0.1: Project Scaffold
**Points**: 2

Initialize Go project with standard structure.

**Tasks**:
- [ ] `go mod init github.com/oladayo21/letterbox`
- [ ] Create directory structure (`cmd/`, `internal/`, `migrations/`)
- [ ] Add Makefile with: `build`, `run`, `test`, `migrate`
- [ ] Add `.gitignore`, `.envrc.example`
- [ ] Add `docker-compose.yml` (postgres + minio for local dev)

**Acceptance**:
- `make build` produces binary
- `make run` starts server (can return 404 for now)
- `docker-compose up -d` starts postgres + minio

**Depends on**: Nothing

---

### Story 0.2: Database Setup + Migrations
**Points**: 2

Setup database connection and migration tooling.

**Tasks**:
- [ ] Add golang-migrate or goose
- [ ] Create initial migration: accounts, emails, attachments, webhooks, webhook_queue tables
- [ ] Add `make migrate-up`, `make migrate-down`, `make migrate-create`
- [ ] Setup sqlc with `sqlc.yaml`
- [ ] Generate initial queries (placeholder)

**Acceptance**:
- `make migrate-up` creates all tables
- `make migrate-down` rolls back cleanly
- sqlc generates without errors

**Depends on**: 0.1

---

### Story 0.3: Configuration & Secrets
**Points**: 1

Load configuration from environment.

**Tasks**:
- [ ] Create `internal/config/config.go`
- [ ] Load: `DATABASE_URL`, `S3_*`, `ENCRYPTION_KEY`, `PORT`
- [ ] Validate required vars on startup
- [ ] Add `internal/crypto/crypto.go` with AES-256 encrypt/decrypt helpers

**Acceptance**:
- App fails fast with clear error if required env vars missing
- `crypto.Encrypt()` / `crypto.Decrypt()` work with test vectors

**Depends on**: 0.1

---

## Epic 1: Account Management
*CRUD for IMAP/SMTP account configuration*

### Story 1.1: Account Model + DB Operations
**Points**: 2

Implement account storage with encrypted credentials.

**Tasks**:
- [ ] Define `Account` struct in `internal/domain/account.go`
- [ ] Write sqlc queries: `CreateAccount`, `GetAccount`, `ListAccounts`, `DeleteAccount`
- [ ] Encrypt passwords before insert, decrypt on read
- [ ] Add `internal/repository/account.go` wrapping sqlc

**Acceptance**:
- Can create account with plaintext password → stored encrypted
- Can read account → password decrypted
- Unit tests pass

**Depends on**: 0.2, 0.3

---

### Story 1.2: Account REST Endpoints
**Points**: 2

Expose account CRUD via REST API.

**Tasks**:
- [ ] Setup Chi/Fiber router in `internal/api/router.go`
- [ ] `POST /accounts` - create account (validate IMAP creds work before saving)
- [ ] `GET /accounts` - list accounts (redact passwords)
- [ ] `GET /accounts/{id}` - get single account
- [ ] `DELETE /accounts/{id}` - remove account
- [ ] Add API key auth middleware (check `X-API-Key` header)

**Acceptance**:
- Can create account via curl, password not returned in response
- Invalid IMAP creds → 400 with clear error
- Requests without valid API key → 401

**Depends on**: 1.1

---

### Story 1.3: IMAP Connection Test
**Points**: 2

Validate IMAP credentials work before saving account.

**Tasks**:
- [ ] Create `internal/imap/client.go` with `TestConnection(host, port, user, pass)`
- [ ] Connect, login, list folders, disconnect
- [ ] Return error with details if connection fails
- [ ] Integrate into `POST /accounts` flow

**Acceptance**:
- Valid creds: account created
- Invalid creds: 400 response, account not saved
- Timeout: handled gracefully with clear message

**Depends on**: 1.1

---

## Epic 2: Email Reading (Core)
*Fetch and store emails from IMAP*

### Story 2.1: Email Model + DB Operations
**Points**: 2

Define email storage with full-text search support.

**Tasks**:
- [ ] Define `Email` struct in `internal/domain/email.go`
- [ ] Write sqlc queries: `InsertEmail`, `GetEmail`, `ListEmails`, `EmailExists`
- [ ] Add GIN index on `search_vector`
- [ ] Create trigger to auto-populate `search_vector` from subject + parsed_text

**Acceptance**:
- Can insert email with all fields
- `search_vector` auto-populated
- Duplicate detection by message_id works

**Depends on**: 0.2

---

### Story 2.2a: Basic Email Parser
**Points**: 2

Parse simple MIME emails (headers + single-part body).

**Tasks**:
- [ ] Create `internal/parser/parser.go`
- [ ] Parse: headers, from/to/cc, subject, date
- [ ] Extract text/plain body
- [ ] Handle basic encoding (quoted-printable, base64)

**Acceptance**:
- Plain text emails parse correctly
- Headers decoded properly
- Unit tests with sample emails

**Depends on**: Nothing (can be done in parallel)

---

### Story 2.2b: Multipart & Attachment Parser
**Points**: 2

Extend parser for complex MIME structures.

**Tasks**:
- [ ] Handle multipart/alternative (text + html)
- [ ] Handle multipart/mixed (body + attachments)
- [ ] Extract attachment metadata (filename, content-type, size, bytes)
- [ ] Handle charset conversions
- [ ] Identify inline images vs attachments

**Acceptance**:
- HTML emails with attachments parse correctly
- Nested multipart structures handled
- Non-ASCII filenames decoded

**Depends on**: 2.2a

---

### Story 2.3: Attachment Storage
**Points**: 2

Upload attachments to S3-compatible storage.

**Tasks**:
- [ ] Create `internal/storage/s3.go` with aws-sdk-go-v2
- [ ] `Upload(bucket, key, data, contentType) → url`
- [ ] `GeneratePresignedURL(key, expiry) → url`
- [ ] Key format: `{account_id}/{email_uid}/{filename}`
- [ ] Integration test with Minio

**Acceptance**:
- Can upload file, get back URL
- Presigned URLs work for download
- Works with Minio locally

**Depends on**: 0.3

---

### Story 2.4a: IMAP Fetch Raw Email
**Points**: 1

Fetch raw email bytes from IMAP by UID.

**Tasks**:
- [ ] Add `FetchRaw(uid, folder) → []byte` to imap client
- [ ] Handle IMAP FETCH command with BODY[]
- [ ] Return raw RFC822 message bytes

**Acceptance**:
- Can fetch any email by UID
- Returns complete raw message
- Handles large emails

**Depends on**: 1.3

---

### Story 2.4b: Email Ingest Pipeline
**Points**: 2

Orchestrate fetch → parse → store flow.

**Tasks**:
- [ ] Create `internal/ingest/ingest.go`
- [ ] Fetch raw via IMAP client
- [ ] Parse using parser
- [ ] Upload attachments to S3
- [ ] Insert email + attachment records to DB
- [ ] Return structured email object

**Acceptance**:
- Single function: `IngestEmail(accountID, folder, uid) → Email`
- Attachments have valid S3 URLs
- Both parsed and raw content stored

**Depends on**: 2.4a, 2.2b, 2.3, 2.1

---

### Story 2.5: List Folders Endpoint
**Points**: 1

List IMAP folders for an account.

**Tasks**:
- [ ] Add `ListFolders()` to imap client
- [ ] `GET /accounts/{id}/folders` → returns folder list with counts

**Acceptance**:
- Returns INBOX, Sent, Drafts, custom folders
- Includes message count per folder

**Depends on**: 1.3

---

### Story 2.6: List Messages Endpoint
**Points**: 2

List emails in a folder (from local mirror).

**Tasks**:
- [ ] `GET /accounts/{id}/folders/{name}/messages`
- [ ] Query params: `limit`, `offset`, `before`, `after`
- [ ] Return email list (headers only, no body)
- [ ] If email not in local DB, trigger on-demand fetch

**Acceptance**:
- Pagination works
- Returns consistent format
- Missing emails trigger fetch

**Depends on**: 2.1, 2.4b

---

### Story 2.7: Get Single Message Endpoint
**Points**: 1

Get full email by UID.

**Tasks**:
- [ ] `GET /accounts/{id}/messages/{uid}`
- [ ] Return full email object (parsed + raw + attachments)
- [ ] Fetch from IMAP if not in local DB

**Acceptance**:
- Returns complete email structure per spec
- Attachment URLs are valid

**Depends on**: 2.4b

---

## Tech Debt: Test Coverage & Error Handling
*Address gaps identified in PR reviews*

### Story TD.1: API & IMAP Test Coverage
**Points**: 2

Add missing test coverage for API handlers and IMAP error classification.

**Tasks**:
- [ ] Add API handler tests: invalid JSON, validation errors, IMAP failures, invalid UUID, not found
- [ ] Add IMAP error classifier tests: `classifyError()`, `isTimeoutError()`, `isTLSError()`
- [ ] Test `classifyImapError()` in API handler

**Acceptance**:
- AccountHandler has >80% test coverage
- IMAP error classification logic fully tested
- No mocked IMAP server required (test error classification only)

**Depends on**: 2.2b

---

### Story TD.2: Error Handling & Type Consolidation
**Points**: 1

Improve error handling and consolidate duplicate types.

**Tasks**:
- [ ] Log IMAP logout errors at debug level instead of discarding
- [ ] Log decryption errors internally before returning generic error
- [ ] Consolidate `parser.EmailAddress` and `domain.EmailAddress` into single type
- [ ] Add List date filter tests (Before/After)

**Acceptance**:
- No silently discarded errors
- Single EmailAddress type used throughout
- Date filter queries tested

**Depends on**: TD.1

---

## Epic 3: Real-time Sync Engine
*Keep local mirror in sync with IMAP*

### Story 3.1a: Single IMAP IDLE Connection
**Points**: 2

Establish and maintain one IDLE connection.

**Tasks**:
- [ ] Create `internal/sync/idle.go`
- [ ] Connect, SELECT folder, enter IDLE mode
- [ ] Parse IDLE responses (EXISTS, EXPUNGE)
- [ ] Emit event channel on new email
- [ ] Detect IDLE capability

**Acceptance**:
- New email → event emitted within seconds
- Non-IDLE servers detected gracefully

**Depends on**: 1.3

---

### Story 3.1b: IDLE Connection Pool + Reconnect
**Points**: 2

Manage multiple IDLE connections with auto-reconnect.

**Tasks**:
- [ ] Create `internal/sync/pool.go`
- [ ] One connection per account
- [ ] Reconnect on disconnect with exponential backoff
- [ ] Add/remove accounts dynamically
- [ ] Health check logging

**Acceptance**:
- Connection drop → auto-reconnect
- Add account → starts IDLE
- Remove account → closes connection

**Depends on**: 3.1a

---

### Story 3.2: Polling Fallback Worker
**Points**: 2

Poll accounts that don't support IDLE.

**Tasks**:
- [ ] Create `internal/sync/poller.go`
- [ ] Check for new emails every 60s (configurable)
- [ ] Track last seen UID per folder
- [ ] Emit same events as IDLE

**Acceptance**:
- New email detected within polling interval
- Doesn't re-emit for already seen emails

**Depends on**: 1.3

---

### Story 3.3: Sync Coordinator
**Points**: 2

Orchestrate IDLE + polling, process new email events.

**Tasks**:
- [ ] Create `internal/sync/coordinator.go`
- [ ] Start IDLE pool + poller on app boot
- [ ] On new email event: fetch, parse, store, notify webhook engine
- [ ] Handle account add/remove at runtime

**Acceptance**:
- New account → sync starts automatically
- Remove account → sync stops
- New emails flow through to storage

**Depends on**: 3.1b, 3.2, 2.4b

---

## Epic 4: Webhook Engine
*Reliable webhook delivery with retries*

### Story 4.1: Webhook Subscription Model
**Points**: 1

Store webhook subscriptions.

**Tasks**:
- [ ] sqlc queries: `CreateWebhook`, `ListWebhooks`, `DeleteWebhook`, `GetWebhooksForAccount`
- [ ] Encrypt webhook secret

**Acceptance**:
- CRUD operations work
- Secrets stored encrypted

**Depends on**: 0.2, 0.3

---

### Story 4.2: Webhook REST Endpoints
**Points**: 1

Manage webhook subscriptions via API.

**Tasks**:
- [ ] `POST /webhooks` - create subscription
- [ ] `GET /webhooks` - list subscriptions
- [ ] `DELETE /webhooks/{id}` - remove subscription

**Acceptance**:
- Can create subscription with URL + secret
- Secret not returned in list response

**Depends on**: 4.1, 1.2

---

### Story 4.3: Webhook Queue Producer
**Points**: 2

Queue webhook deliveries when new emails arrive.

**Tasks**:
- [ ] On new email stored, find matching webhook subscriptions
- [ ] Build payload (full email, handle attachment size threshold)
- [ ] Insert into `webhook_queue` with status=pending

**Acceptance**:
- New email → queue entry created for each subscription
- Attachments >1MB → use S3 URL instead of inline

**Depends on**: 4.1, 3.3

---

### Story 4.4a: Webhook Delivery Worker
**Points**: 2

Basic webhook delivery without retries.

**Tasks**:
- [ ] Create `internal/webhook/worker.go`
- [ ] Poll `webhook_queue` for pending items
- [ ] POST payload to webhook URL
- [ ] On success: mark delivered
- [ ] On failure: mark failed (no retry yet)

**Acceptance**:
- Successful delivery → status=delivered
- Failed delivery → status=failed
- Worker runs continuously

**Depends on**: 4.3

---

### Story 4.4b: Webhook Retry with Backoff
**Points**: 1

Add exponential backoff retry logic.

**Tasks**:
- [ ] On failure: increment attempts, calculate next_attempt
- [ ] Backoff: 1m, 5m, 15m, 1h, 4h
- [ ] Max 5 attempts, then mark permanently failed
- [ ] Query filters by `next_attempt <= now()`

**Acceptance**:
- 500 response → retries with increasing delays
- After 5 failures → permanently failed
- Logs show retry schedule

**Depends on**: 4.4a

---

### Story 4.5: Webhook Signature Verification
**Points**: 1

Sign payloads for receivers to verify.

**Tasks**:
- [ ] HMAC-SHA256 of payload using webhook secret
- [ ] Add `X-Letterbox-Signature` header
- [ ] Add `X-Letterbox-Timestamp` for replay protection
- [ ] Document verification in README

**Acceptance**:
- Signature matches when verified with secret
- Timestamp within 5 min tolerance

**Depends on**: 4.4

---

## Epic 5: Search
*Full-text search across emails*

### Story 5.1: Search Query Builder
**Points**: 2

Build Postgres FTS queries.

**Tasks**:
- [ ] Create `internal/search/search.go`
- [ ] Parse user query → `plainto_tsquery` or `websearch_to_tsquery`
- [ ] Support: basic terms, phrases, AND/OR

**Acceptance**:
- `"invoice"` → finds emails with invoice
- `"john smith"` → phrase search works

**Depends on**: 2.1

---

### Story 5.2: Search Endpoint
**Points**: 1

Expose search via REST.

**Tasks**:
- [ ] `GET /search?q=...&account_id=...`
- [ ] Return paginated email list (headers only)
- [ ] Highlight matching terms (optional)

**Acceptance**:
- Search returns relevant results
- Pagination works
- Scoped to account

**Depends on**: 5.1

---

## Epic 6: Production Readiness
*Polish for real usage*

### Story 6.1: Structured Logging
**Points**: 1

Add proper logging throughout.

**Tasks**:
- [ ] Add slog or zerolog
- [ ] Log levels: debug, info, warn, error
- [ ] Structured fields: account_id, email_uid, webhook_id
- [ ] Request logging middleware

**Acceptance**:
- All operations logged with context
- JSON output for production

**Depends on**: Nothing (can do anytime)

---

### Story 6.2: Health Endpoints
**Points**: 1

Add health and readiness checks.

**Tasks**:
- [ ] `GET /health` - basic liveness
- [ ] `GET /ready` - check DB, S3, IMAP pool status

**Acceptance**:
- `/health` returns 200 if process alive
- `/ready` returns 503 if any dependency down

**Depends on**: 0.1

---

### Story 6.3: Graceful Shutdown
**Points**: 1

Clean shutdown of all components.

**Tasks**:
- [ ] Handle SIGTERM/SIGINT
- [ ] Close IMAP connections gracefully
- [ ] Drain webhook queue worker
- [ ] Close DB connections

**Acceptance**:
- `kill` → clean shutdown log, no orphaned connections

**Depends on**: 3.3, 4.4b

---

### Story 6.4: Docker Image
**Points**: 1

Build production Docker image.

**Tasks**:
- [ ] Multi-stage Dockerfile
- [ ] Embed migrations in binary or copy to image
- [ ] Document env vars in README

**Acceptance**:
- `docker build` produces working image
- `docker run` starts service

**Depends on**: All

---

## Story Order (Critical Path)

```
0.1 → 0.2 → 0.3 → 1.1 → 1.2 → 1.3
                              ↓
      2.2a → 2.2b ──────→ 2.4b → 2.6 → 2.7
      2.3 ────────────────↗
      2.4a ───────────────↗
                              ↓
                  3.1a → 3.1b → 3.3
                  3.2 ─────────↗ ↓
                          4.1 → 4.2 → 4.3 → 4.4a → 4.4b → 4.5
                                                    ↓
                                             5.1 → 5.2
                                                    ↓
                                             6.1-6.4
```

## Quick Reference

| Story | Points | Can Start After |
|-------|--------|-----------------|
| 0.1   | 2      | -               |
| 0.2   | 2      | 0.1             |
| 0.3   | 1      | 0.1             |
| 1.1   | 2      | 0.2, 0.3        |
| 1.2   | 2      | 1.1             |
| 1.3   | 2      | 1.1             |
| 2.1   | 2      | 0.2             |
| 2.2a  | 2      | -               |
| 2.2b  | 2      | 2.2a            |
| 2.3   | 2      | 0.3             |
| 2.4a  | 1      | 1.3             |
| 2.4b  | 2      | 2.4a, 2.2b, 2.3, 2.1 |
| 2.5   | 1      | 1.3             |
| 2.6   | 2      | 2.1, 2.4b       |
| 2.7   | 1      | 2.4b            |
| 3.1a  | 2      | 1.3             |
| 3.1b  | 2      | 3.1a            |
| 3.2   | 2      | 1.3             |
| 3.3   | 2      | 3.1b, 3.2, 2.4b |
| 4.1   | 1      | 0.2, 0.3        |
| 4.2   | 1      | 4.1, 1.2        |
| 4.3   | 2      | 4.1, 3.3        |
| 4.4a  | 2      | 4.3             |
| 4.4b  | 1      | 4.4a            |
| 4.5   | 1      | 4.4b            |
| 5.1   | 2      | 2.1             |
| 5.2   | 1      | 5.1             |
| 6.1   | 1      | -               |
| 6.2   | 1      | 0.1             |
| 6.3   | 1      | 3.3, 4.4b       |
| 6.4   | 1      | All             |

**Total**: 51 story points (31 stories, max 2 points each)

## Definition of Done

Each story is complete when:
- [ ] Code written and compiles
- [ ] Unit tests pass
- [ ] Integration test (where applicable)
- [ ] Endpoint documented in README (if API)
- [ ] Works with `make run` locally
