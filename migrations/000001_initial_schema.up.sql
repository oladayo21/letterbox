-- Initial schema for letterbox
-- Based on SPEC.md data model

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- accounts table
CREATE TABLE accounts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            TEXT NOT NULL,
    imap_host       TEXT NOT NULL,
    imap_port       INT NOT NULL,
    imap_user       TEXT NOT NULL,
    imap_password   TEXT NOT NULL,  -- encrypted
    smtp_host       TEXT,
    smtp_port       INT,
    smtp_user       TEXT,
    smtp_password   TEXT,           -- encrypted
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- emails table
CREATE TABLE emails (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id          UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    uid                 INT NOT NULL,
    message_id          TEXT,
    folder              TEXT NOT NULL,
    subject             TEXT,
    from_email          TEXT,
    from_name           TEXT,
    date                TIMESTAMP WITH TIME ZONE,
    parsed_text         TEXT,
    parsed_html         TEXT,
    raw                 TEXT,
    flags               TEXT[],
    deleted_upstream    BOOLEAN DEFAULT false,
    search_vector       TSVECTOR,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(account_id, folder, uid)
);

-- Full-text search index
CREATE INDEX emails_search_idx ON emails USING GIN(search_vector);

-- Index for common queries
CREATE INDEX emails_account_folder_idx ON emails(account_id, folder, date DESC);
CREATE INDEX emails_message_id_idx ON emails(message_id) WHERE message_id IS NOT NULL;

-- Trigger to auto-populate search_vector
CREATE OR REPLACE FUNCTION emails_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.subject, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.from_name, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.from_email, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.parsed_text, '')), 'C');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER emails_search_vector_trigger
    BEFORE INSERT OR UPDATE ON emails
    FOR EACH ROW EXECUTE FUNCTION emails_search_vector_update();

-- attachments table
CREATE TABLE attachments (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email_id        UUID NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    size            INT NOT NULL,
    s3_key          TEXT NOT NULL
);

CREATE INDEX attachments_email_id_idx ON attachments(email_id);

-- webhooks table
CREATE TABLE webhooks (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    url             TEXT NOT NULL,
    secret          TEXT NOT NULL,  -- encrypted
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX webhooks_account_id_idx ON webhooks(account_id);

-- webhook_queue table
CREATE TABLE webhook_queue (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    webhook_id      UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    email_id        UUID NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    payload         JSONB NOT NULL,
    attempts        INT DEFAULT 0,
    next_attempt    TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX webhook_queue_pending_idx ON webhook_queue(next_attempt) WHERE status = 'pending';
CREATE INDEX webhook_queue_webhook_id_idx ON webhook_queue(webhook_id);
