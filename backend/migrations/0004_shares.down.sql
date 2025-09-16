--0004_shares.down.sql
DROP INDEX IF EXISTS idx_share_users_user;
DROP INDEX IF EXISTS idx_share_users_share;
DROP TABLE IF EXISTS share_users;

DROP INDEX IF EXISTS idx_shares_target;
DROP TABLE IF EXISTS shares;