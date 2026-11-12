-- 1Click quotes and their funding status (funding_quotes).

-- name: InsertFundingQuote :exec
INSERT INTO funding_quotes (
    id, escrow_id, origin_asset, refund_to, deposit_address, deposit_memo, amount_in, amount_in_formatted,
    amount_out, deadline, quote, status, watching)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: GetFundingQuote :one
SELECT *
FROM funding_quotes
WHERE id = $1;

-- name: GetLatestFundingQuote :one
SELECT *
FROM funding_quotes
WHERE escrow_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: UpdateFundingStatus :execrows
UPDATE funding_quotes
SET status = $2, oneclick_status = $3, watching = $4, checked_at = now(), updated_at = now()
WHERE id = $1;

-- name: ListWatchedQuotes :many
SELECT id
FROM funding_quotes
WHERE watching AND (checked_at IS NULL OR checked_at < sqlc.arg(checked_before)::timestamptz)
ORDER BY checked_at NULLS FIRST, id
LIMIT sqlc.arg(max_rows);
