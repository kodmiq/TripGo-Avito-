-- +goose Up
-- Частичный уникальный индекс, а не проверка в коде: два параллельных INSERT
-- на одного водителя не могут оба пройти. Таблица на этом шаге пустая,
-- CONCURRENTLY не нужен.
CREATE UNIQUE INDEX trips_one_active_driver_idx
    ON trips (driver_id)
    WHERE status = 'active';

-- +goose Down
DROP INDEX trips_one_active_driver_idx;
