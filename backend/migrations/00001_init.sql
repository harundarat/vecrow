-- Baseline migration: the schema starts empty. Each table is added by its
-- own migration (make migrate-create name=<name>).

-- +goose Up
SELECT 1;

-- +goose Down
SELECT 1;
