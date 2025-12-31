-- Revert BIGINT to INT (may fail if data exceeds INT range)

ALTER TABLE emails ALTER COLUMN uid TYPE INT;
ALTER TABLE attachments ALTER COLUMN size TYPE INT;
