-- name: CreateWebhook :one
INSERT INTO webhooks (
    account_id, url, secret
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: GetWebhook :one
SELECT * FROM webhooks WHERE id = $1;

-- name: ListWebhooks :many
SELECT * FROM webhooks ORDER BY created_at DESC;

-- name: GetWebhooksForAccount :many
SELECT * FROM webhooks WHERE account_id = $1 ORDER BY created_at DESC;

-- name: DeleteWebhook :execrows
DELETE FROM webhooks WHERE id = $1;
