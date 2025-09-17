-- 0006_admin.down.sql

DROP INDEX IF EXISTS idx_audit_logs_user;
DROP INDEX IF EXISTS idx_audit_logs_created_at;

DROP TABLE IF EXISTS audit_logs;

DROP INDEX IF EXISTS idx_users_is_admin;

ALTER TABLE users
DROP COLUMN IF EXISTS is_admin;