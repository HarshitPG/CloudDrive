-- 0004_shares.up.sql
CREATE TABLE IF NOT EXISTS shares (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token TEXT UNIQUE NOT NULL,               
  creator_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  target_type TEXT NOT NULL,               
  target_id UUID NOT NULL,                 
  title TEXT,
  description TEXT,
  expires_at timestamptz NULL,
  revoked BOOLEAN NOT NULL DEFAULT FALSE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_shares_target ON shares (target_type, target_id);

CREATE TABLE IF NOT EXISTS share_users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  share_id UUID NOT NULL REFERENCES shares(id) ON DELETE CASCADE,
  target_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  permission TEXT NOT NULL DEFAULT 'read',  -- 'read' | 'write' (future)
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_share_users_share ON share_users (share_id);
CREATE INDEX IF NOT EXISTS idx_share_users_user ON share_users (target_user_id);
