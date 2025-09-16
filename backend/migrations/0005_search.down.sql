-- 0005_search.down.sql 
-- Reverts the changes from the search migration

-- Drop all created indexes
DROP INDEX IF EXISTS idx_user_files_search;
DROP INDEX IF EXISTS idx_user_files_filename_trgm;
DROP INDEX IF EXISTS idx_user_files_size;
DROP INDEX IF EXISTS idx_user_files_created_at;

-- Drop the trigger on the user_files table
DROP TRIGGER IF EXISTS trg_user_files_search ON user_files;

-- Drop the trigger function
DROP FUNCTION IF EXISTS user_files_search_trigger();

-- Drop the helper functions
DROP FUNCTION IF EXISTS normalize_filename(text);
DROP FUNCTION IF EXISTS jsonb_array_to_string(jsonb);

-- Drop the tsvector column from the user_files table
ALTER TABLE user_files
DROP COLUMN IF EXISTS search_document;

-- Drop the trigram extension
DROP EXTENSION IF EXISTS pg_trgm;