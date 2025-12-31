-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1;

-- name: ListAccounts :many
SELECT * FROM accounts ORDER BY created_at DESC;

-- name: CreateAccount :one
INSERT INTO accounts (
    name, imap_host, imap_port, imap_user, imap_password,
    smtp_host, smtp_port, smtp_user, smtp_password
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: DeleteAccount :exec
DELETE FROM accounts WHERE id = $1;
