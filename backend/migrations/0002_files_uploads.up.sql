-- 0002_files_uploads.up.sql
CREATE TABLE IF NOT EXISTS file_contents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  content_hash TEXT UNIQUE NOT NULL,
  blob_key TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  mime_type TEXT,
  ref_count BIGINT NOT NULL DEFAULT 0,
  metadata JSONB DEFAULT '{}' ,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_file_contents_content_hash ON file_contents (content_hash);

CREATE TABLE IF NOT EXISTS user_files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content_id UUID NOT NULL REFERENCES file_contents(id) ON DELETE RESTRICT,
  filename TEXT NOT NULL,
  declared_mime TEXT,
  original_size_bytes BIGINT NOT NULL,
  folder_id UUID NULL,
  is_public BOOLEAN DEFAULT FALSE,
  public_token TEXT,
  tags JSONB DEFAULT '[]'::jsonb,
  version_of UUID NULL,
  download_count BIGINT DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);

CREATE INDEX IF NOT EXISTS idx_user_files_user_id ON user_files (user_id);

CREATE TABLE IF NOT EXISTS upload_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  declared_mime TEXT,
  original_size_bytes BIGINT NOT NULL,
  temp_blob_key TEXT NOT NULL, 
  client_sha256 TEXT NULL,
  status TEXT NOT NULL DEFAULT 'OPEN',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_upload_sessions_user ON upload_sessions (user_id);
