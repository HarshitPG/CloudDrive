-- migrations/0001_init.down.sql

-- Drop dependent table first
DROP TABLE IF EXISTS refresh_tokens;

-- Drop users table
DROP TABLE IF EXISTS users;

-- Optionally, you can drop the extension if it's not used by anything else
-- DROP EXTENSION IF EXISTS "pgcrypto";