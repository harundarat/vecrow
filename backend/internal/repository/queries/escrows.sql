-- Several-array inserts unnest each array in the select list, which
-- PostgreSQL zips row by row (the arrays have the same length).

-- Escrows and their milestones, submissions, parties, transactions and
-- events (migrations/00003_create_escrows.sql).

-- name: InsertEscrow :exec
INSERT INTO escrows (id, creator_account_id, creator_role, status, fee_bps, freelancer, split, approver, refund_account)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertMilestones :exec
INSERT INTO escrow_milestones (escrow_id, milestone, amount)
SELECT sqlc.arg(escrow_id)::text, (m.ord - 1)::integer, m.amount
FROM unnest(sqlc.arg(amounts)::text[]) WITH ORDINALITY AS m (amount, ord);

-- name: GetEscrow :one
SELECT *
FROM escrows
WHERE id = $1;

-- name: LockEscrow :one
SELECT *
FROM escrows
WHERE id = $1
FOR UPDATE;

-- name: ListMilestones :many
SELECT milestone, amount, status, submitted_at_ms, revision_count
FROM escrow_milestones
WHERE escrow_id = $1
ORDER BY milestone;

-- name: GetActiveSubmission :one
SELECT *
FROM escrow_submissions
WHERE escrow_id = $1 AND status IN ('pending', 'approved', 'created');

-- name: ListEscrowPendingPayouts :many
SELECT account_id, amount
FROM escrow_pending_payouts
WHERE escrow_id = $1
ORDER BY account_id;

-- name: UpdateEscrowStatus :execrows
UPDATE escrows
SET status = $2, updated_at = now()
WHERE id = $1;

-- Returns no row when the escrow already has an active submission.
-- name: InsertSubmission :one
INSERT INTO escrow_submissions (
    id, escrow_id, status, terms, message, deadline_ms, wallet_scheme, wallet_address,
    counterparty_signature, counterparty_public_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: UpdateSubmission :execrows
UPDATE escrow_submissions
SET status = $2, creator_signature = $3, creator_public_key = $4, failure_reason = $5, updated_at = now()
WHERE id = $1;

-- name: DeleteParties :exec
DELETE FROM escrow_parties
WHERE escrow_id = $1;

-- name: InsertParties :exec
INSERT INTO escrow_parties (escrow_id, role, scheme, address)
SELECT sqlc.arg(escrow_id)::text, unnest(sqlc.arg(roles)::text[]), unnest(sqlc.arg(schemes)::text[]),
       unnest(sqlc.arg(addresses)::text[])
ON CONFLICT DO NOTHING;

-- Stores a get_escrow snapshot unless a newer one is stored. Returns no row
-- when the stored snapshot was read at a higher block.
-- name: UpdateEscrowChain :one
UPDATE escrows
SET status = $2, review_window_sec = $3, balance = $4, chain_created_at_ms = $5, funded_at_ms = $6,
    chain_block_height = $7, chain_synced_at = now(), updated_at = now()
WHERE id = $1 AND (chain_block_height IS NULL OR chain_block_height <= $7)
RETURNING id;

-- submitted_at_ms is -1 for a milestone that has none.
-- name: UpdateMilestonesChain :exec
UPDATE escrow_milestones AS m
SET status = c.status, submitted_at_ms = NULLIF(c.submitted_at_ms, -1), revision_count = c.revision_count
FROM (SELECT unnest(sqlc.arg(milestones)::integer[]) AS milestone,
             unnest(sqlc.arg(statuses)::text[]) AS status,
             unnest(sqlc.arg(submitted_at_ms)::bigint[]) AS submitted_at_ms,
             unnest(sqlc.arg(revision_counts)::integer[]) AS revision_count) AS c
WHERE m.escrow_id = sqlc.arg(escrow_id)::text AND m.milestone = c.milestone;

-- name: DeletePendingPayouts :exec
DELETE FROM escrow_pending_payouts
WHERE escrow_id = $1;

-- name: InsertPendingPayouts :exec
INSERT INTO escrow_pending_payouts (escrow_id, account_id, amount)
SELECT sqlc.arg(escrow_id)::text, unnest(sqlc.arg(account_ids)::text[]), unnest(sqlc.arg(amounts)::text[]);

-- Returns no row for a transaction already stored.
-- name: InsertTransaction :one
INSERT INTO escrow_transactions (tx_hash, escrow_id, method, signer_id, sender_id, receiver_id, succeeded, error)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tx_hash) DO NOTHING
RETURNING tx_hash;

-- name: InsertEvent :exec
INSERT INTO escrow_events (receipt_id, log_index, tx_hash, position, escrow_id, event, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING;

-- name: ListTransactions :many
SELECT tx_hash, escrow_id, method, signer_id, sender_id, receiver_id, succeeded, error, created_at
FROM escrow_transactions
WHERE escrow_id = $1
ORDER BY seq;

-- name: ListEvents :many
SELECT e.receipt_id, e.log_index, e.tx_hash, e.event, e.data
FROM escrow_events e
JOIN escrow_transactions t ON t.tx_hash = e.tx_hash
WHERE t.escrow_id = $1
ORDER BY t.seq, e.position;

-- name: InsertRevisionNote :exec
INSERT INTO revision_notes (id, escrow_id, milestone, reason, tx_hash)
VALUES ($1, $2, $3, $4, $5);

-- name: ListRevisionNotes :many
SELECT milestone, reason, tx_hash, created_at
FROM revision_notes
WHERE escrow_id = $1
ORDER BY created_at, id;

-- The escrows in which an account has a role: through a linked wallet
-- named in the escrow, or as its creator. Filters are optional.
-- name: ListAccountEscrows :many
WITH roles AS (
    SELECT p.escrow_id, p.role
    FROM escrow_parties p
    JOIN wallets w ON w.scheme = p.scheme AND w.address = p.address
    WHERE w.account_id = sqlc.arg(account_id)
    UNION
    SELECT e.id, e.creator_role
    FROM escrows e
    WHERE e.creator_account_id = sqlc.arg(account_id)
)
SELECT e.id, array_agg(DISTINCT r.role ORDER BY r.role)::text[] AS roles
FROM escrows e
JOIN roles r ON r.escrow_id = e.id
WHERE sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status)::text
GROUP BY e.id, e.created_at
HAVING sqlc.narg(role)::text IS NULL OR sqlc.narg(role)::text = ANY (array_agg(r.role))
ORDER BY e.created_at DESC, e.id
LIMIT sqlc.arg(max_rows);

-- name: ListApprovedEscrows :many
SELECT id
FROM escrows
WHERE status = 'Approved'
ORDER BY updated_at, id
LIMIT $1;

-- name: ListExpiredSubmissions :many
SELECT escrow_id
FROM escrow_submissions
WHERE status = 'pending' AND deadline_ms <= $1
ORDER BY deadline_ms
LIMIT $2;

-- Submitted milestones of funded escrows whose review window has ended.
-- name: ListDueReleases :many
SELECT m.escrow_id, m.milestone
FROM escrow_milestones m
JOIN escrows e ON e.id = m.escrow_id
WHERE e.status = 'Funded' AND m.status = 'Submitted' AND e.review_window_sec IS NOT NULL
  AND m.submitted_at_ms + e.review_window_sec * 1000 <= sqlc.arg(now_ms)::bigint
ORDER BY m.submitted_at_ms, m.escrow_id, m.milestone
LIMIT sqlc.arg(max_rows);

-- name: ListDuePayouts :many
SELECT escrow_id, account_id
FROM escrow_pending_payouts
ORDER BY escrow_id, account_id
LIMIT $1;

-- Escrows on-chain that can still change: Created, Funded, or with pending
-- payouts, last synced before older_than.
-- name: ListEscrowsToSync :many
SELECT e.id
FROM escrows e
WHERE e.chain_created_at_ms IS NOT NULL
  AND (e.status IN ('Created', 'Funded')
       OR EXISTS (SELECT 1 FROM escrow_pending_payouts p WHERE p.escrow_id = e.id))
  AND (e.chain_synced_at IS NULL OR e.chain_synced_at < sqlc.arg(older_than)::timestamptz)
ORDER BY e.chain_synced_at NULLS FIRST, e.id
LIMIT sqlc.arg(max_rows);
