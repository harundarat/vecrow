-- Escrow invitations (spec.md R-15).

-- The invitation of an escrow, with the current username of an invited account.
-- name: GetInvitation :one
SELECT i.kind, i.account_id, a.username, i.email, i.wallet_scheme, i.wallet_address
FROM escrow_invitations i
LEFT JOIN accounts a ON a.id = i.account_id
WHERE i.escrow_id = $1;

-- name: UpsertInvitation :exec
INSERT INTO escrow_invitations (escrow_id, kind, account_id, email, wallet_scheme, wallet_address)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (escrow_id) DO UPDATE
SET kind = EXCLUDED.kind, account_id = EXCLUDED.account_id, email = EXCLUDED.email,
    wallet_scheme = EXCLUDED.wallet_scheme, wallet_address = EXCLUDED.wallet_address, updated_at = now();

-- name: DeleteInvitation :exec
DELETE FROM escrow_invitations
WHERE escrow_id = $1;
