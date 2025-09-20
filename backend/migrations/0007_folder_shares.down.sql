-- Remove folder shares enhancements
ALTER TABLE shares DROP COLUMN IF EXISTS snapshot_mode;
ALTER TABLE shares DROP COLUMN IF EXISTS recursive;