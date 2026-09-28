ALTER TABLE channel_contributions ADD COLUMN settings jsonb NOT NULL DEFAULT '{}';
CREATE TABLE channel_contribution_rpm (
    account_id bigint PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    window_start timestamptz NOT NULL,
    used integer NOT NULL DEFAULT 0
);
CREATE TABLE channel_contribution_batch_holds (
    batch_id text PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts(id),
    amount numeric(20,8) NOT NULL CHECK (amount >= 0),
    settled boolean NOT NULL DEFAULT false
);
CREATE INDEX channel_contribution_batch_holds_account_idx ON channel_contribution_batch_holds(account_id) WHERE NOT settled;
