-- Inserts skip conflicts instead of failing: a unique violation aborts the
-- surrounding transaction in PostgreSQL, so callers could not retry inside it.

-- name: InsertAccount :one
INSERT INTO accounts (id, username)
VALUES ($1, $2)
ON CONFLICT (username) DO NOTHING
RETURNING id;

-- name: GetAccount :one
SELECT id, username, created_at
FROM accounts
WHERE id = $1;

-- name: LockAccount :one
SELECT id
FROM accounts
WHERE id = $1
FOR UPDATE;

-- name: UpdateUsername :execrows
UPDATE accounts
SET username = $2, updated_at = now()
WHERE id = $1;

-- name: InsertWallet :one
INSERT INTO wallets (id, account_id, scheme, address)
VALUES ($1, $2, $3, $4)
ON CONFLICT (scheme, address) DO NOTHING
RETURNING id;

-- name: GetWalletOwner :one
SELECT account_id
FROM wallets
WHERE scheme = $1 AND address = $2;

-- name: ListWallets :many
SELECT id, scheme, address, created_at
FROM wallets
WHERE account_id = $1
ORDER BY created_at, id;

-- name: CountWallets :one
SELECT count(*)
FROM wallets
WHERE account_id = $1;

-- name: DeleteWallet :execrows
DELETE FROM wallets
WHERE id = $1 AND account_id = $2;
