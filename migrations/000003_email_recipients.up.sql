-- Add to/cc recipient columns as JSONB arrays
-- Format: [{"name": "John", "email": "john@example.com"}, ...]

ALTER TABLE emails ADD COLUMN to_recipients JSONB DEFAULT '[]'::jsonb;
ALTER TABLE emails ADD COLUMN cc_recipients JSONB DEFAULT '[]'::jsonb;
