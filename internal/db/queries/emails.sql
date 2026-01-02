-- name: InsertEmail :one
INSERT INTO emails (
    account_id, uid, message_id, folder, subject,
    from_email, from_name, to_recipients, cc_recipients,
    date, parsed_text, parsed_html, raw, flags
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: GetEmail :one
SELECT * FROM emails WHERE id = $1;

-- name: GetEmailByUID :one
SELECT * FROM emails
WHERE account_id = $1 AND folder = $2 AND uid = $3;

-- name: ListEmails :many
SELECT * FROM emails
WHERE account_id = $1
  AND folder = $2
  AND (sqlc.narg('before')::timestamptz IS NULL OR date < sqlc.narg('before'))
  AND (sqlc.narg('after')::timestamptz IS NULL OR date > sqlc.narg('after'))
ORDER BY date DESC
LIMIT $3 OFFSET $4;

-- name: EmailExistsByUID :one
SELECT EXISTS(
    SELECT 1 FROM emails
    WHERE account_id = $1 AND folder = $2 AND uid = $3
) AS exists;

-- name: EmailExistsByMessageID :one
SELECT EXISTS(
    SELECT 1 FROM emails
    WHERE account_id = $1 AND message_id = $2
) AS exists;

-- name: UpdateEmailFlags :execrows
UPDATE emails SET flags = $2 WHERE id = $1;

-- name: MarkEmailDeletedUpstream :execrows
UPDATE emails SET deleted_upstream = true WHERE id = $1;

-- name: CountEmailsInFolder :one
SELECT COUNT(*) FROM emails
WHERE account_id = $1 AND folder = $2 AND deleted_upstream = false;

-- name: SearchEmails :many
SELECT * FROM emails
WHERE account_id = $1
  AND deleted_upstream = false
  AND search_vector @@ websearch_to_tsquery('english', $2)
  AND (sqlc.narg('folder')::text IS NULL OR folder = sqlc.narg('folder'))
ORDER BY date DESC
LIMIT $3 OFFSET $4;

-- name: CountSearchEmails :one
SELECT COUNT(*) FROM emails
WHERE account_id = $1
  AND deleted_upstream = false
  AND search_vector @@ websearch_to_tsquery('english', $2)
  AND (sqlc.narg('folder')::text IS NULL OR folder = sqlc.narg('folder'));
