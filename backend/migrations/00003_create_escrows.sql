-- Escrows (spec.md R-16, R-20, R-21, R-23; architecture.md §7.2). Until an
-- escrow is Created on-chain, these rows are its only record. Afterwards the
-- contract is the source of truth: the status, milestones, balance and
-- pending payouts here are a snapshot read with get_escrow, kept so jobs can
-- find due work. Transactions, events, revision notes and funding quotes are
-- what the contract does not keep.

-- +goose Up
CREATE TABLE escrows (
    -- The escrow ID used on-chain: a lowercase UUID chosen at creation.
    id                  text        PRIMARY KEY,
    creator_account_id  uuid        NOT NULL REFERENCES accounts (id),
    creator_role        text        NOT NULL,
    status              text        NOT NULL,
    fee_bps             integer     NOT NULL,
    -- The creator's own details: the freelancer account and split, or the
    -- approver wallet and refund account.
    freelancer          text,
    split               jsonb,
    approver            jsonb,
    refund_account      text,
    -- Snapshot of get_escrow once the escrow is on-chain.
    review_window_sec   bigint,
    balance             text,
    chain_created_at_ms bigint,
    funded_at_ms        bigint,
    chain_block_height  bigint,
    chain_synced_at     timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT escrows_id_format CHECK (id ~ '^[a-z0-9_-]{1,64}$'),
    CONSTRAINT escrows_creator_role_check CHECK (creator_role IN ('freelancer', 'client')),
    CONSTRAINT escrows_status_check CHECK (status IN (
        'Open', 'PendingApproval', 'Approved', 'Created', 'Funded', 'Completed', 'Cancelled')),
    CONSTRAINT escrows_fee_bps_check CHECK (fee_bps BETWEEN 0 AND 10000),
    CONSTRAINT escrows_balance_format CHECK (balance ~ '^[0-9]+$'),
    CONSTRAINT escrows_creator_details CHECK (
        (creator_role = 'freelancer' AND freelancer IS NOT NULL AND split IS NOT NULL
            AND approver IS NULL AND refund_account IS NULL)
        OR (creator_role = 'client' AND approver IS NOT NULL AND refund_account IS NOT NULL
            AND freelancer IS NULL AND split IS NULL))
);

CREATE INDEX escrows_creator_account_idx ON escrows (creator_account_id, created_at DESC);
CREATE INDEX escrows_status_idx ON escrows (status);

-- Milestone amounts as proposed, then their on-chain state.
CREATE TABLE escrow_milestones (
    escrow_id       text    NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    milestone       integer NOT NULL,
    amount          text    NOT NULL,
    status          text    NOT NULL DEFAULT 'Pending',
    submitted_at_ms bigint,
    revision_count  integer NOT NULL DEFAULT 0,
    PRIMARY KEY (escrow_id, milestone),
    CONSTRAINT escrow_milestones_amount_format CHECK (amount ~ '^[1-9][0-9]*$'),
    CONSTRAINT escrow_milestones_status_check CHECK (status IN (
        'Pending', 'Submitted', 'RevisionRequested', 'Released', 'Refunded'))
);

CREATE INDEX escrow_milestones_submitted_idx ON escrow_milestones (submitted_at_ms)
    WHERE status = 'Submitted';

-- The other party's signed submission of the final terms, and the creator's
-- signature over the same terms once approved. At most one is active.
CREATE TABLE escrow_submissions (
    id                      uuid        PRIMARY KEY,
    escrow_id               text        NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    status                  text        NOT NULL,
    -- The terms in the JSON form create_escrow takes, and the message signed.
    terms                   jsonb       NOT NULL,
    message                 text        NOT NULL,
    deadline_ms             bigint      NOT NULL,
    -- The wallet that submitted: the freelancer account or the approver.
    wallet_scheme           text        NOT NULL,
    wallet_address          text        NOT NULL,
    counterparty_signature  text        NOT NULL,
    counterparty_public_key text,
    creator_signature       text,
    creator_public_key      text,
    failure_reason          text,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT escrow_submissions_status_check CHECK (status IN (
        'pending', 'approved', 'created', 'rejected', 'expired', 'failed', 'cancelled')),
    CONSTRAINT escrow_submissions_creator_signature CHECK (
        status NOT IN ('approved', 'created') OR creator_signature IS NOT NULL)
);

CREATE UNIQUE INDEX escrow_submissions_active_key ON escrow_submissions (escrow_id)
    WHERE status IN ('pending', 'approved', 'created');
CREATE INDEX escrow_submissions_pending_idx ON escrow_submissions (deadline_ms)
    WHERE status = 'pending';

-- Wallets that have a role in an escrow, for "My escrows": the freelancer
-- account, split members and the approver, from the creator's details and the
-- active submission. Rewritten whenever either changes.
CREATE TABLE escrow_parties (
    escrow_id text NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    role      text NOT NULL,
    scheme    text NOT NULL,
    address   text NOT NULL,
    PRIMARY KEY (escrow_id, role, scheme, address),
    CONSTRAINT escrow_parties_role_check CHECK (role IN ('freelancer', 'client', 'member'))
);

CREATE INDEX escrow_parties_wallet_idx ON escrow_parties (scheme, address);

-- Payouts the contract holds as pending because the transfer failed.
CREATE TABLE escrow_pending_payouts (
    escrow_id  text NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    account_id text NOT NULL,
    amount     text NOT NULL,
    PRIMARY KEY (escrow_id, account_id),
    CONSTRAINT escrow_pending_payouts_amount_format CHECK (amount ~ '^[1-9][0-9]*$')
);

-- Transactions the backend sent for an escrow, with their outcome.
CREATE TABLE escrow_transactions (
    tx_hash     text        PRIMARY KEY,
    -- Insertion order: transactions are listed in the order they were sent.
    seq         bigint      GENERATED ALWAYS AS IDENTITY,
    escrow_id   text        NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    method      text        NOT NULL,
    -- signer_id paid the gas; sender_id is the account the call acts as (the
    -- freelancer for a meta transaction, otherwise the signer).
    signer_id   text        NOT NULL,
    sender_id   text        NOT NULL,
    receiver_id text        NOT NULL,
    succeeded   boolean     NOT NULL,
    error       text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX escrow_transactions_escrow_idx ON escrow_transactions (escrow_id, seq);

-- NEP-297 events logged by the escrow contract in those transactions.
CREATE TABLE escrow_events (
    receipt_id text        NOT NULL,
    log_index  integer     NOT NULL,
    tx_hash    text        NOT NULL REFERENCES escrow_transactions (tx_hash) ON DELETE CASCADE,
    -- The event's position among the transaction's events, in execution order.
    position   integer     NOT NULL,
    escrow_id  text        NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    event      text        NOT NULL,
    data       jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (receipt_id, log_index)
);

CREATE INDEX escrow_events_tx_idx ON escrow_events (tx_hash, position);
CREATE INDEX escrow_events_escrow_idx ON escrow_events (escrow_id);

-- Reasons given with revision requests; the contract stores only the count.
CREATE TABLE revision_notes (
    id         uuid        PRIMARY KEY,
    escrow_id  text        NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    milestone  integer     NOT NULL,
    reason     text        NOT NULL,
    tx_hash    text        NOT NULL REFERENCES escrow_transactions (tx_hash),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX revision_notes_escrow_idx ON revision_notes (escrow_id, created_at);

-- 1Click quotes requested by the approver, and the funding status watched
-- until the swap ends.
CREATE TABLE funding_quotes (
    id                  uuid        PRIMARY KEY,
    escrow_id           text        NOT NULL REFERENCES escrows (id) ON DELETE CASCADE,
    origin_asset        text        NOT NULL,
    refund_to           text        NOT NULL,
    deposit_address     text        NOT NULL,
    deposit_memo        text,
    amount_in           text        NOT NULL,
    amount_in_formatted text        NOT NULL,
    amount_out          text        NOT NULL,
    deadline            timestamptz,
    quote               jsonb       NOT NULL,
    status              text        NOT NULL DEFAULT 'PENDING',
    oneclick_status     text,
    watching            boolean     NOT NULL DEFAULT true,
    checked_at          timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT funding_quotes_status_check CHECK (status IN ('PENDING', 'SUCCESS', 'REFUNDED'))
);

CREATE INDEX funding_quotes_escrow_idx ON funding_quotes (escrow_id, created_at DESC);
CREATE INDEX funding_quotes_watching_idx ON funding_quotes (created_at) WHERE watching;

-- +goose Down
DROP TABLE funding_quotes;
DROP TABLE revision_notes;
DROP TABLE escrow_events;
DROP TABLE escrow_transactions;
DROP TABLE escrow_pending_payouts;
DROP TABLE escrow_parties;
DROP TABLE escrow_submissions;
DROP TABLE escrow_milestones;
DROP TABLE escrows;
