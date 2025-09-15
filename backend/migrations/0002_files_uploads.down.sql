-- 0002_files_uploads.down.sql

DROP INDEX IF EXISTS idx_upload_sessions_user;
DROP TABLE IF EXISTS upload_sessions;

DROP INDEX IF EXISTS idx_user_files_user_id;
DROP TABLE IF EXISTS user_files;

DROP INDEX IF EXISTS idx_file_contents_content_hash;
DROP TABLE IF EXISTS file_contents;