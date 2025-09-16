-- 0002_folders.down.sql

DROP INDEX IF EXISTS idx_folders_parent;
DROP INDEX IF EXISTS idx_folders_user;

DROP TABLE IF EXISTS folders;