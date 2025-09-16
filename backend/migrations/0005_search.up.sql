-- Weighted full-text search support for user_files (filename A, tags B)
ALTER TABLE user_files
  ADD COLUMN IF NOT EXISTS search_document tsvector;

-- helper: aggregate JSONB tags into a string
CREATE OR REPLACE FUNCTION jsonb_array_to_string(j jsonb) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT coalesce(string_agg(elem, ' '), '')
  FROM jsonb_array_elements_text(j) AS elem;
$$;

-- helper: normalize filenames by replacing separators
CREATE OR REPLACE FUNCTION normalize_filename(txt text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT lower(regexp_replace(txt, '[-_.]', ' ', 'g'));
$$;

-- initial population of the search column for existing rows
UPDATE user_files
SET search_document =
  setweight(to_tsvector('simple', normalize_filename(coalesce(filename, ''))), 'A') ||
  setweight(to_tsvector('simple', coalesce(jsonb_array_to_string(tags), '')), 'B');

-- trigger to automatically update the search column
CREATE OR REPLACE FUNCTION user_files_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_document :=
    setweight(to_tsvector('simple', normalize_filename(coalesce(NEW.filename,''))),'A') ||
    setweight(to_tsvector('simple', coalesce(jsonb_array_to_string(NEW.tags),'')),'B');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_user_files_search ON user_files;
CREATE TRIGGER trg_user_files_search
BEFORE INSERT OR UPDATE ON user_files
FOR EACH ROW EXECUTE PROCEDURE user_files_search_trigger();

-- GIN index for Full-Text Search (FTS)
CREATE INDEX IF NOT EXISTS idx_user_files_search
  ON user_files USING GIN (search_document);

-- Trigram extension and index for partial filename search
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_user_files_filename_trgm
  ON user_files USING GIN (normalize_filename(filename) gin_trgm_ops);

-- extra indexes for common filters
CREATE INDEX IF NOT EXISTS idx_user_files_size ON user_files (original_size_bytes);
CREATE INDEX IF NOT EXISTS idx_user_files_created_at ON user_files (created_at);