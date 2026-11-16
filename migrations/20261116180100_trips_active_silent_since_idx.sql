-- +goose NO TRANSACTION
-- +goose Up
-- Выборка устаревших поездок фильтрует status = active и сравнивает
-- COALESCE(last_position_at, started_at). Частичный индекс по этому выражению
-- даёт index scan и на первой порции, и на продолжении по (время, id).
CREATE INDEX CONCURRENTLY IF NOT EXISTS trips_active_silent_since_idx
    ON trips ((COALESCE(last_position_at, started_at)), id)
    WHERE status = 'active';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS trips_active_silent_since_idx;
