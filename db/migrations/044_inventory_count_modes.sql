-- +goose Up
ALTER TABLE inventory_counts
  ADD COLUMN count_mode text NOT NULL DEFAULT 'open' CHECK (count_mode IN ('open','blind')),
  ADD COLUMN area text NOT NULL DEFAULT '' CHECK (length(area) <= 120);
GRANT INSERT(count_mode,area) ON inventory_counts TO mekuin_app;

-- +goose Down
ALTER TABLE inventory_counts DROP COLUMN area;
ALTER TABLE inventory_counts DROP COLUMN count_mode;
