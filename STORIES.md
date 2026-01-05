# letterbox - Development Stories

Stories ordered for sequential implementation. Each story is independently deployable where possible.

---

## Epic 0: Project Foundation ✅
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

## Epic 1: Account Management ✅
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

## Epic 2: Email Reading (Core) ✅
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

## Epic 3: Real-time Sync Engine ✅
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

## Epic 4: Webhook Engine ✅
*Reliable webhook delivery with retries*

### Story 4.1: Webhook Subscription Model ✅
**Points**: 1

Store webhook subscriptions.

**Tasks**:
- [x] sqlc queries: `CreateWebhook`, `ListWebhooks`, `DeleteWebhook`, `GetWebhooksForAccount`
- [x] Encrypt webhook secret

**Acceptance**:
- CRUD operations work
- Secrets stored encrypted

**Depends on**: 0.2, 0.3

---

### Story 4.2: Webhook REST Endpoints ✅
**Points**: 1

Manage webhook subscriptions via API.

**Tasks**:
- [x] `POST /webhooks` - create subscription
- [x] `GET /webhooks` - list subscriptions
- [x] `DELETE /webhooks/{id}` - remove subscription

**Acceptance**:
- Can create subscription with URL + secret
- Secret not returned in list response

**Depends on**: 4.1, 1.2

---

### Story 4.3: Webhook Queue Producer ✅
**Points**: 2

Queue webhook deliveries when new emails arrive.

**Tasks**:
- [x] On new email stored, find matching webhook subscriptions
- [x] Build payload (full email, handle attachment size threshold)
- [x] Insert into `webhook_queue` with status=pending

**Acceptance**:
- New email → queue entry created for each subscription
- Attachments use S3 presigned URLs

**Depends on**: 4.1, 3.3

---

### Story 4.4a: Webhook Delivery Worker ✅
**Points**: 2

Basic webhook delivery without retries.

**Tasks**:
- [x] Create `internal/webhook/worker.go`
- [x] Poll `webhook_queue` for pending items
- [x] POST payload to webhook URL
- [x] On success: mark delivered
- [x] On failure: mark failed (no retry yet)

**Acceptance**:
- Successful delivery → status=delivered
- Failed delivery → status=failed
- Worker runs continuously

**Depends on**: 4.3

---

### Story 4.4b: Webhook Retry with Backoff ✅
**Points**: 1

Add exponential backoff retry logic.

**Tasks**:
- [x] On failure: increment attempts, calculate next_attempt
- [x] Backoff: 1m, 5m, 15m, 1h, 4h
- [x] Max 5 attempts, then mark permanently failed
- [x] Query filters by `next_attempt <= now()`

**Acceptance**:
- 500 response → retries with increasing delays
- After 5 failures → permanently failed
- Logs show retry schedule

**Depends on**: 4.4a

---

### Story 4.5: Webhook Signature Verification ✅
**Points**: 1

Sign payloads for receivers to verify.

**Tasks**:
- [x] HMAC-SHA256 of payload using webhook secret
- [x] Add `X-Letterbox-Signature` header
- [x] Add `X-Letterbox-Timestamp` for replay protection
- [x] Document verification in README

**Acceptance**:
- Signature matches when verified with secret
- Timestamp within 5 min tolerance

**Depends on**: 4.4

---

## Epic 5: Search ✅
*Full-text search across emails*

### Story 5.1: Search Query Builder ✅
**Points**: 2

Build Postgres FTS queries.

**Tasks**:
- [x] Create `internal/search/search.go`
- [x] Parse user query → `plainto_tsquery` or `websearch_to_tsquery`
- [x] Support: basic terms, phrases, AND/OR

**Acceptance**:
- `"invoice"` → finds emails with invoice
- `"john smith"` → phrase search works

**Depends on**: 2.1

---

### Story 5.2: Search Endpoint ✅
**Points**: 1

Expose search via REST.

**Tasks**:
- [x] `GET /search?q=...&account_id=...`
- [x] Return paginated email list (headers only)
- [x] Highlight matching terms (optional)

**Acceptance**:
- Search returns relevant results
- Pagination works
- Scoped to account

**Depends on**: 5.1

---

## Epic 6: Production Readiness ✅
*Polish for real usage*

### Story 6.1: Structured Logging ✅
**Points**: 1

Add proper logging throughout.

**Tasks**:
- [x] Add slog or zerolog
- [x] Log levels: debug, info, warn, error
- [x] Structured fields: account_id, email_uid, webhook_id
- [x] Request logging middleware

**Acceptance**:
- All operations logged with context
- JSON output for production

**Depends on**: Nothing (can do anytime)

---

### Story 6.2: Health Endpoints ✅
**Points**: 1

Add health and readiness checks.

**Tasks**:
- [x] `GET /health` - basic liveness
- [x] `GET /ready` - check DB, S3, IMAP pool status

**Acceptance**:
- `/health` returns 200 if process alive
- `/ready` returns 503 if any dependency down

**Depends on**: 0.1

---

### Story 6.3: Graceful Shutdown ✅
**Points**: 1

Clean shutdown of all components.

**Tasks**:
- [x] Handle SIGTERM/SIGINT
- [x] Close IMAP connections gracefully
- [x] Drain webhook queue worker
- [x] Close DB connections

**Acceptance**:
- `kill` → clean shutdown log, no orphaned connections

**Depends on**: 3.3, 4.4b

---

### Story 6.4: Docker Image ✅
**Points**: 1

Build production Docker image.

**Tasks**:
- [x] Multi-stage Dockerfile
- [x] Embed migrations in binary or copy to image
- [x] Document env vars in README

**Acceptance**:
- `docker build` produces working image
- `docker run` starts service

**Depends on**: All

---

## Phase 1 Story Order (Critical Path) - COMPLETE ✅

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

## Phase 1 Quick Reference (COMPLETE ✅)

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

**Phase 1 Total**: 51 story points (31 stories) - ALL COMPLETE ✅

## Definition of Done

Each story is complete when:
- [ ] Code written and compiles
- [ ] Unit tests pass
- [ ] Integration test (where applicable)
- [ ] Endpoint documented in README (if API)
- [ ] Works with `make run` locally

---

# Phase 2: Extended Features

---

## Epic 7: SMTP Send ✅
*Send emails via configured SMTP accounts*

### Story 7.1: SMTP Client ✅
**Points**: 2

Create SMTP client for sending emails.

**Tasks**:
- [x] Create `internal/smtp/client.go`
- [x] Support STARTTLS and direct TLS
- [x] `SendEmail(host, port, user, pass, message) → error`
- [x] Connection test helper (similar to IMAP)
- [x] Handle common auth mechanisms (PLAIN, LOGIN)

**Acceptance**:
- Can send email through Gmail, Outlook, Fastmail
- TLS handshake works correctly
- Invalid creds → clear error message

**Depends on**: 0.3

---

### Story 7.2: SMTP Connection Test on Account Create ✅
**Points**: 1

Validate SMTP credentials when adding account.

**Tasks**:
- [x] Extend `POST /accounts` to validate SMTP creds if provided
- [x] Test connection without sending (EHLO + AUTH only)
- [x] Return specific error for IMAP vs SMTP failures

**Acceptance**:
- Invalid SMTP creds → 400 with `smtp_error` field
- Can create account with IMAP-only (SMTP optional)

**Depends on**: 7.1, 1.2

---

### Story 7.3: Compose Email Model ✅
**Points**: 1

Define email composition structure.

**Tasks**:
- [x] Create `ComposeEmail` struct (to, cc, bcc, subject, text, html, attachments)
- [x] Validate email addresses
- [x] Build RFC 2822 compliant message with MIME

**Acceptance**:
- Plain text email builds correctly
- HTML + attachments build as multipart/mixed

**Depends on**: 2.2b

---

### Story 7.4: Send Email Endpoint ✅
**Points**: 2

Expose email sending via REST.

**Tasks**:
- [x] `POST /accounts/{id}/messages` - send new email
- [x] Accept JSON body with recipients, subject, body, attachments
- [x] Store sent email in local DB (mirror Sent folder)
- [x] Return message ID on success

**Acceptance**:
- Can send plain text email via API
- Can send HTML email with attachments
- Sent email appears in local mirror

**Depends on**: 7.2, 7.3

---

### Story 7.5: Reply & Forward Support ✅
**Points**: 2

Handle reply threading and forwarding.

**Tasks**:
- [x] Accept `reply_to` or `forward` parameter with original message UID
- [x] Set `In-Reply-To` and `References` headers for threading
- [x] Auto-quote original message in body (configurable)
- [x] Forward: include original attachments option

**Acceptance**:
- Reply appears in same thread in email clients
- Forward includes original content
- `References` header chains correctly

**Depends on**: 7.4

---

## Epic 8: Email Operations
*Move, delete, and flag emails*

### Story 8.1: Flag Operations
**Points**: 2

Mark emails as read/unread, flagged/unflagged.

**Tasks**:
- [ ] Add `SetFlags(uid, folder, flags)` to IMAP client
- [ ] `PATCH /accounts/{id}/messages/{uid}` with `flags` array
- [ ] Support: `\Seen`, `\Flagged`, `\Answered`, `\Draft`
- [ ] Update local DB after successful IMAP operation

**Acceptance**:
- Mark as read → `\Seen` flag set on IMAP server
- Local DB stays in sync
- Invalid flag → 400 error

**Depends on**: 1.3, 2.1

---

### Story 8.2: Move Email
**Points**: 2

Move emails between folders.

**Tasks**:
- [ ] Add `MoveEmail(uid, srcFolder, destFolder)` to IMAP client
- [ ] `PATCH /accounts/{id}/messages/{uid}` with `folder` field
- [ ] Use IMAP MOVE command (or COPY + DELETE for older servers)
- [ ] Update local DB folder reference

**Acceptance**:
- Move email to Archive → appears in Archive folder
- Original UID may change (handle in response)
- Non-existent destination → 400 error

**Depends on**: 1.3, 2.1

---

### Story 8.3: Delete Email
**Points**: 2

Delete (trash) emails.

**Tasks**:
- [ ] Add `DeleteEmail(uid, folder, permanent)` to IMAP client
- [ ] `DELETE /accounts/{id}/messages/{uid}?permanent=false`
- [ ] Default: move to Trash folder
- [ ] Permanent: set `\Deleted` flag and EXPUNGE
- [ ] Update local DB (`deleted_upstream = true`)

**Acceptance**:
- Soft delete → email in Trash folder
- Permanent delete → removed from server
- Local DB marked as deleted (not removed)

**Depends on**: 8.2

---

### Story 8.4: Bulk Operations
**Points**: 2

Handle multiple emails in single request.

**Tasks**:
- [ ] `PATCH /accounts/{id}/messages` with array of operations
- [ ] `DELETE /accounts/{id}/messages` with array of UIDs
- [ ] Batch IMAP operations for efficiency
- [ ] Return partial success results

**Acceptance**:
- Can mark 100 emails as read in one request
- Partial failures return which operations succeeded
- IMAP connection reused for batch

**Depends on**: 8.1, 8.2, 8.3

---

## Epic 9: Webhook Filters
*Filter which emails trigger webhooks*

### Story 9.1: Webhook Filter Model
**Points**: 1

Define filter criteria for webhooks.

**Tasks**:
- [ ] Add `filters` JSONB column to webhooks table
- [ ] Filter schema: `{folders: [], from_patterns: [], subject_patterns: []}`
- [ ] Migration to add column

**Acceptance**:
- Can store filter criteria
- Null filters = all emails (backward compatible)

**Depends on**: 4.1

---

### Story 9.2: Folder Filter
**Points**: 1

Filter webhooks by email folder.

**Tasks**:
- [ ] Extend webhook subscription to accept `folders` array
- [ ] Only trigger webhook if email folder matches
- [ ] Empty array = all folders

**Acceptance**:
- Webhook with `folders: ["INBOX"]` only fires for INBOX
- Multiple folders work as OR condition

**Depends on**: 9.1

---

### Story 9.3: Sender Pattern Filter
**Points**: 2

Filter webhooks by sender email/domain.

**Tasks**:
- [ ] Accept `from_patterns` array (glob patterns)
- [ ] Support: exact match, domain match (`*@example.com`), wildcard
- [ ] Case-insensitive matching

**Acceptance**:
- `*@github.com` matches all GitHub notifications
- `john@*` matches john from any domain
- Multiple patterns work as OR

**Depends on**: 9.1

---

### Story 9.4: Subject Pattern Filter
**Points**: 2

Filter webhooks by subject line.

**Tasks**:
- [ ] Accept `subject_patterns` array (glob patterns)
- [ ] Support: contains, starts with, regex (optional)
- [ ] Case-insensitive by default

**Acceptance**:
- `*invoice*` matches subjects containing "invoice"
- `[GitHub]*` matches GitHub notification subjects
- Combined with sender filter works as AND

**Depends on**: 9.1

---

### Story 9.5: Filter Evaluation Engine
**Points**: 2

Evaluate filters when producing webhook events.

**Tasks**:
- [ ] Create `internal/webhook/filter.go`
- [ ] Evaluate all filter criteria (AND logic between types)
- [ ] Short-circuit evaluation for performance
- [ ] Update producer to check filters before queueing

**Acceptance**:
- Email matches filters → webhook queued
- Email doesn't match → no webhook
- Multiple webhooks with different filters → each evaluated independently

**Depends on**: 9.2, 9.3, 9.4, 4.3

---

## Epic 10: History Backfill
*Import historical emails from IMAP*

### Story 10.1: Backfill Command
**Points**: 2

CLI command to backfill historical emails.

**Tasks**:
- [ ] Add `letterbox backfill --account-id=X --folder=INBOX --since=2024-01-01`
- [ ] Paginate through IMAP SEARCH results
- [ ] Respect rate limits (configurable delay between fetches)
- [ ] Resume support (track progress in DB)

**Acceptance**:
- Can backfill last 30 days of INBOX
- Interrupted backfill can resume
- Progress logged

**Depends on**: 2.4b

---

### Story 10.2: Backfill Progress Tracking
**Points**: 1

Track and report backfill progress.

**Tasks**:
- [ ] Create `backfill_jobs` table (account_id, folder, status, progress)
- [ ] Update progress after each batch
- [ ] `GET /accounts/{id}/backfill` - check status

**Acceptance**:
- Can see "500/2000 emails imported"
- Completed backfill shows 100%
- Failed backfill shows error and resume point

**Depends on**: 10.1

---

### Story 10.3: Backfill API Trigger
**Points**: 1

Trigger backfill via REST API.

**Tasks**:
- [ ] `POST /accounts/{id}/backfill` with folder, since date
- [ ] Runs in background (returns job ID)
- [ ] Webhook notification on completion (optional)

**Acceptance**:
- Can trigger backfill from API
- Non-blocking (returns immediately)
- Job status queryable

**Depends on**: 10.1, 10.2

---

# Phase 3: Multi-tenancy & Scale

---

## Epic 11: Multi-user & API Keys
*Support multiple users with separate API keys*

### Story 11.1: User Model
**Points**: 2

Define user entity and storage.

**Tasks**:
- [ ] Create `users` table (id, email, name, created_at)
- [ ] Add `user_id` foreign key to accounts table
- [ ] Migration with backward compatibility (existing accounts → default user)
- [ ] sqlc queries for user CRUD

**Acceptance**:
- Users can be created
- Accounts are scoped to users
- Existing data migrated cleanly

**Depends on**: 0.2

---

### Story 11.2: API Key Model
**Points**: 2

Manage multiple API keys per user.

**Tasks**:
- [ ] Create `api_keys` table (id, user_id, key_hash, name, scopes, expires_at)
- [ ] Hash API keys (never store plaintext)
- [ ] Support scopes: `read`, `write`, `admin`
- [ ] Key rotation support (create new, revoke old)

**Acceptance**:
- Can create API key, only shown once
- Keys can have expiration
- Scopes limit operations

**Depends on**: 11.1

---

### Story 11.3: API Key Authentication
**Points**: 2

Replace single API key with multi-key auth.

**Tasks**:
- [ ] Look up key in `api_keys` table (by hash)
- [ ] Inject user context into request
- [ ] Check scopes for each endpoint
- [ ] Rate limit per key (prep for Epic 12)

**Acceptance**:
- Old single API key still works (migration path)
- New keys work with proper scoping
- Invalid key → 401, insufficient scope → 403

**Depends on**: 11.2, 1.2

---

### Story 11.4: API Key Management Endpoints
**Points**: 2

CRUD for API keys.

**Tasks**:
- [ ] `POST /api-keys` - create new key (returns plaintext once)
- [ ] `GET /api-keys` - list keys (metadata only, no secrets)
- [ ] `DELETE /api-keys/{id}` - revoke key
- [ ] Require admin scope for key management

**Acceptance**:
- Can create and revoke API keys via API
- Key shown only on creation response
- Revoked key immediately stops working

**Depends on**: 11.3

---

### Story 11.5: User Isolation
**Points**: 2

Ensure users can only access their own data.

**Tasks**:
- [ ] Add user_id filter to all account queries
- [ ] Add user_id filter to webhook queries
- [ ] Add user_id filter to search queries
- [ ] Audit existing endpoints for isolation

**Acceptance**:
- User A cannot see User B's accounts
- Cross-user access → 404 (not 403, for security)
- Superadmin can access all (optional)

**Depends on**: 11.3

---

## Epic 12: Rate Limiting
*Protect service from abuse and overload*

### Story 12.1: Rate Limiter Core
**Points**: 2

Implement token bucket rate limiting.

**Tasks**:
- [ ] Create `internal/ratelimit/limiter.go`
- [ ] Token bucket algorithm with Redis backend
- [ ] Fallback to in-memory for single-instance
- [ ] Configurable limits per key type

**Acceptance**:
- Can limit to X requests per minute
- Bursting allowed within limits
- Distributed rate limiting works (Redis)

**Depends on**: 0.3

---

### Story 12.2: Rate Limit Middleware
**Points**: 1

Apply rate limits to API endpoints.

**Tasks**:
- [ ] Create rate limit middleware
- [ ] Different limits: per API key, per endpoint, per IP
- [ ] Return `429 Too Many Requests` with `Retry-After` header
- [ ] Log rate limit hits

**Acceptance**:
- Excessive requests → 429
- `Retry-After` header accurate
- Headers show remaining quota (`X-RateLimit-*`)

**Depends on**: 12.1

---

### Story 12.3: IMAP Connection Limits
**Points**: 2

Limit concurrent IMAP connections per account.

**Tasks**:
- [ ] Track active connections per account
- [ ] Queue excess connection requests
- [ ] Timeout queued requests after configurable period
- [ ] Prevent single account from exhausting pool

**Acceptance**:
- Max 3 concurrent connections per account
- 4th request waits, then executes
- No deadlocks in pool

**Depends on**: 3.1b, 12.1

---

### Story 12.4: Webhook Rate Limiting
**Points**: 1

Limit webhook delivery rate per subscription.

**Tasks**:
- [ ] Max deliveries per minute per webhook URL
- [ ] Queue excess deliveries
- [ ] Configurable per webhook (optional)

**Acceptance**:
- Burst of 100 emails → delivered over time, not instantly
- Prevents overwhelming webhook receivers
- No lost webhooks

**Depends on**: 4.4a, 12.1

---

## Epic 13: OAuth Provider Support
*Support OAuth-based email providers*

### Story 13.1: OAuth Token Model
**Points**: 2

Store and refresh OAuth tokens.

**Tasks**:
- [ ] Add `oauth_tokens` table (account_id, provider, access_token, refresh_token, expires_at)
- [ ] Encrypt tokens at rest
- [ ] Token refresh logic with retry

**Acceptance**:
- Tokens stored encrypted
- Auto-refresh before expiry
- Failed refresh → alert/notification

**Depends on**: 0.3

---

### Story 13.2: Gmail OAuth Flow
**Points**: 3

Implement Gmail OAuth 2.0.

**Tasks**:
- [ ] Register OAuth app with Google
- [ ] `GET /oauth/gmail/authorize` - redirect to Google consent
- [ ] `GET /oauth/gmail/callback` - exchange code for tokens
- [ ] Use XOAUTH2 for IMAP authentication

**Acceptance**:
- Can connect Gmail account via OAuth
- No password stored, only tokens
- Token refresh works automatically

**Depends on**: 13.1

---

### Story 13.3: Outlook OAuth Flow
**Points**: 3

Implement Microsoft OAuth 2.0.

**Tasks**:
- [ ] Register OAuth app with Azure AD
- [ ] `GET /oauth/outlook/authorize` - redirect to Microsoft consent
- [ ] `GET /oauth/outlook/callback` - exchange code for tokens
- [ ] Use XOAUTH2 for IMAP authentication

**Acceptance**:
- Can connect Outlook/Office 365 account via OAuth
- Token refresh works
- Personal and work accounts supported

**Depends on**: 13.1

---

### Story 13.4: OAuth Account Creation
**Points**: 2

Create accounts from OAuth callbacks.

**Tasks**:
- [ ] Detect provider from email domain (optional auto-suggest)
- [ ] `POST /accounts` accepts `oauth_code` instead of password
- [ ] Store tokens, not password
- [ ] IMAP client uses XOAUTH2 when tokens present

**Acceptance**:
- Can create account with just OAuth flow
- IMAP connection works with token auth
- Clear error if provider not supported

**Depends on**: 13.2, 13.3, 1.2

---

### Story 13.5: Token Refresh Worker
**Points**: 2

Background worker to refresh expiring tokens.

**Tasks**:
- [ ] Check for tokens expiring in next 30 minutes
- [ ] Refresh tokens proactively
- [ ] Retry failed refreshes with backoff
- [ ] Alert on permanent refresh failure (user needs to re-auth)

**Acceptance**:
- Tokens refreshed before expiry
- IMAP connections never fail due to expired token
- Failed refresh → notification/webhook

**Depends on**: 13.1

---

# Phase 2 & 3 Story Order

```
Phase 2:
7.1 → 7.2 → 7.3 → 7.4 → 7.5 (SMTP)
                    ↓
8.1 → 8.2 → 8.3 → 8.4 (Operations)
                    ↓
9.1 → 9.2/9.3/9.4 → 9.5 (Webhook Filters)
                    ↓
10.1 → 10.2 → 10.3 (Backfill)

Phase 3:
11.1 → 11.2 → 11.3 → 11.4 → 11.5 (Multi-user)
                          ↓
12.1 → 12.2 → 12.3/12.4 (Rate Limiting)
                          ↓
13.1 → 13.2/13.3 → 13.4 → 13.5 (OAuth)
```

## Phase 2 & 3 Quick Reference

| Story | Points | Can Start After |
|-------|--------|-----------------|
| 7.1   | 2      | 0.3             |
| 7.2   | 1      | 7.1, 1.2        |
| 7.3   | 1      | 2.2b            |
| 7.4   | 2      | 7.2, 7.3        |
| 7.5   | 2      | 7.4             |
| 8.1   | 2      | 1.3, 2.1        |
| 8.2   | 2      | 1.3, 2.1        |
| 8.3   | 2      | 8.2             |
| 8.4   | 2      | 8.1, 8.2, 8.3   |
| 9.1   | 1      | 4.1             |
| 9.2   | 1      | 9.1             |
| 9.3   | 2      | 9.1             |
| 9.4   | 2      | 9.1             |
| 9.5   | 2      | 9.2, 9.3, 9.4, 4.3 |
| 10.1  | 2      | 2.4b            |
| 10.2  | 1      | 10.1            |
| 10.3  | 1      | 10.1, 10.2      |
| 11.1  | 2      | 0.2             |
| 11.2  | 2      | 11.1            |
| 11.3  | 2      | 11.2, 1.2       |
| 11.4  | 2      | 11.3            |
| 11.5  | 2      | 11.3            |
| 12.1  | 2      | 0.3             |
| 12.2  | 1      | 12.1            |
| 12.3  | 2      | 3.1b, 12.1      |
| 12.4  | 1      | 4.4a, 12.1      |
| 13.1  | 2      | 0.3             |
| 13.2  | 3      | 13.1            |
| 13.3  | 3      | 13.1            |
| 13.4  | 2      | 13.2, 13.3, 1.2 |
| 13.5  | 2      | 13.1            |

**Phase 2 Total**: 24 story points (15 stories)
**Phase 3 Total**: 30 story points (15 stories)
**Grand Total (All Phases)**: 105 story points (61 stories)
