-- An optional email per account (spec.md R-14), verified with a one-time
-- code before it is used: only the verified email matches invitations (R-15)
-- and allows recovery logins (R-22). A new email waits in pending_email until
-- its code is checked, so the verified email keeps working meanwhile.

-- +goose Up
ALTER TABLE accounts
    ADD COLUMN email             text,
    ADD COLUMN email_verified_at timestamptz,
    ADD COLUMN pending_email     text,
    ADD CONSTRAINT accounts_email_key UNIQUE (email),
    ADD CONSTRAINT accounts_email_lower CHECK (email = lower(email)),
    ADD CONSTRAINT accounts_pending_email_lower CHECK (pending_email = lower(pending_email)),
    ADD CONSTRAINT accounts_email_verified CHECK ((email IS NULL) = (email_verified_at IS NULL));

-- +goose Down
ALTER TABLE accounts
    DROP COLUMN pending_email,
    DROP COLUMN email_verified_at,
    DROP COLUMN email;
