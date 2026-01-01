-- name: InsertAttachment :one
INSERT INTO attachments (
    email_id, filename, content_type, size, s3_key
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetAttachmentsByEmailID :many
SELECT * FROM attachments WHERE email_id = $1;

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = $1;

-- name: DeleteAttachmentsByEmailID :exec
DELETE FROM attachments WHERE email_id = $1;
