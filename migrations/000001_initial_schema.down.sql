-- Rollback initial schema

DROP TRIGGER IF EXISTS emails_search_vector_trigger ON emails;
DROP FUNCTION IF EXISTS emails_search_vector_update();

DROP TABLE IF EXISTS webhook_queue;
DROP TABLE IF EXISTS webhooks;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS emails;
DROP TABLE IF EXISTS accounts;

DROP EXTENSION IF EXISTS "uuid-ossp";
