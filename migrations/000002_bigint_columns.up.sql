-- Change INT to BIGINT for columns that may overflow

-- emails.uid: IMAP UIDs are unsigned 32-bit, postgres INT is signed 32-bit
ALTER TABLE emails ALTER COLUMN uid TYPE BIGINT;

-- attachments.size: files can exceed 2GB
ALTER TABLE attachments ALTER COLUMN size TYPE BIGINT;
