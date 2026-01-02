-- name: CreateWebhookQueueItem :one
INSERT INTO webhook_queue (
    webhook_id, email_id, payload, status
) VALUES (
    $1, $2, $3, 'pending'
) RETURNING *;

-- name: GetWebhookQueueItem :one
SELECT * FROM webhook_queue WHERE id = $1;

-- name: GetPendingWebhookQueueItems :many
SELECT * FROM webhook_queue
WHERE status = 'pending' AND next_attempt <= NOW()
ORDER BY created_at ASC
LIMIT $1;

-- name: UpdateWebhookQueueStatus :execrows
UPDATE webhook_queue
SET status = $2, attempts = $3, next_attempt = $4
WHERE id = $1;

-- name: MarkWebhookQueueDelivered :execrows
UPDATE webhook_queue
SET status = 'delivered'
WHERE id = $1;

-- name: MarkWebhookQueueFailed :execrows
UPDATE webhook_queue
SET status = 'failed'
WHERE id = $1;
