-- Accounts and the wallets linked to them (spec.md R-13, R-14). An account is
-- backend identity only; the contract never reads it (D-08).

-- +goose Up
CREATE TABLE accounts (
    id         uuid        PRIMARY KEY,
    username   text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounts_username_key UNIQUE (username),
    CONSTRAINT accounts_username_format CHECK (username ~ '^[a-z][a-z0-9_]{2,29}$')
);

-- A wallet belongs to at most one account: the unique key makes a second
-- login with the same wallet open the same account, and rejects linking a
-- wallet that another account already owns.
CREATE TABLE wallets (
    id         uuid        PRIMARY KEY,
    account_id uuid        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    scheme     text        NOT NULL,
    address    text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT wallets_scheme_address_key UNIQUE (scheme, address),
    CONSTRAINT wallets_scheme_check CHECK (scheme IN ('nep413', 'eip191', 'ed25519'))
);

CREATE INDEX wallets_account_id_idx ON wallets (account_id);

-- +goose Down
DROP TABLE wallets;
DROP TABLE accounts;
