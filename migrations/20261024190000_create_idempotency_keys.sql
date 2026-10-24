-- +goose Up
-- Ключ резервируется до вставки поездки, поэтому внешний ключ проверяется
-- в конце транзакции, а не сразу.
CREATE TABLE idempotency_keys (
    idempotency_key UUID PRIMARY KEY,
    request_hash    TEXT NOT NULL,
    trip_id         UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT idempotency_keys_trip_id_fkey
        FOREIGN KEY (trip_id) REFERENCES trips(id)
        ON DELETE CASCADE
        DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);

-- +goose Down
DROP TABLE idempotency_keys;
