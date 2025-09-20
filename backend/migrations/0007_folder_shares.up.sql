-- Add recursive support to existing shares table
ALTER TABLE shares ADD COLUMN IF NOT EXISTS recursive BOOLEAN DEFAULT FALSE;
ALTER TABLE shares ADD COLUMN IF NOT EXISTS snapshot_mode BOOLEAN DEFAULT FALSE;

-- Add index for performance
CREATE INDEX IF NOT EXISTS idx_shares_recursive ON shares(recursive) WHERE recursive = true;