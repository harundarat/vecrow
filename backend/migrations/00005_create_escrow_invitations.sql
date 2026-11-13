-- Invitations (spec.md R-15): the creator of an escrow names the other party
-- by username, email or wallet so the escrow appears in their account (R-16).
-- An invitation grants visibility only; the terms never take an address from
-- it (D-08).
--
-- An invitation by username stores the account, found when it is made. One by
-- email or wallet stores the email or wallet, matched against verified emails
-- and linked wallets whenever an account's escrows are listed, so it reaches
-- the account that verifies the email or links the wallet later.

-- +goose Up
CREATE TABLE escrow_invitations (
    escrow_id      text        PRIMARY KEY REFERENCES escrows (id) ON DELETE CASCADE,
    kind           text        NOT NULL,
    account_id     uuid        REFERENCES accounts (id) ON DELETE CASCADE,
    email          text,
    wallet_scheme  text,
    wallet_address text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT escrow_invitations_kind_check CHECK (
        (kind = 'username' AND account_id IS NOT NULL
            AND email IS NULL AND wallet_scheme IS NULL AND wallet_address IS NULL)
        OR (kind = 'email' AND email IS NOT NULL
            AND account_id IS NULL AND wallet_scheme IS NULL AND wallet_address IS NULL)
        OR (kind = 'wallet' AND wallet_scheme IS NOT NULL AND wallet_address IS NOT NULL
            AND account_id IS NULL AND email IS NULL)),
    CONSTRAINT escrow_invitations_email_lower CHECK (email = lower(email)),
    CONSTRAINT escrow_invitations_wallet_scheme_check CHECK (wallet_scheme IN ('nep413', 'eip191', 'ed25519'))
);

CREATE INDEX escrow_invitations_account_idx ON escrow_invitations (account_id)
    WHERE account_id IS NOT NULL;
CREATE INDEX escrow_invitations_email_idx ON escrow_invitations (email)
    WHERE email IS NOT NULL;
CREATE INDEX escrow_invitations_wallet_idx ON escrow_invitations (wallet_scheme, wallet_address)
    WHERE wallet_scheme IS NOT NULL;

-- +goose Down
DROP TABLE escrow_invitations;
