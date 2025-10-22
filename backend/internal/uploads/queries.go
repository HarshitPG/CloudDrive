package uploads

const (
	// Folder operations
	qGetFolderByPath = `
		SELECT id FROM folders 
		WHERE user_id=$1 
		  AND (parent_id = NULLIF($2,'')::uuid OR (parent_id IS NULL AND $2=''))
		  AND name=$3 
		  AND deleted_at IS NULL 
		LIMIT 1`

	qInsertFolder = `
		INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, NULLIF($2,'')::uuid, $3, now(), now())
		RETURNING id`

	// File content deduplication
	qCheckContentExists = `SELECT id, size_bytes FROM file_contents WHERE content_hash=$1 LIMIT 1`

	qInsertExistingFileRef = `
		INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6,'')::uuid, now(), now())
		RETURNING id`

	qIncRefCount = `UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1`

	// Upload session operations
	qInsertUploadSession = `
		INSERT INTO upload_sessions (id, user_id, filename, declared_mime, original_size_bytes, temp_blob_key, client_sha256, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'OPEN',now(),now())`

	qGetUploadSession = `
		SELECT temp_blob_key, filename, declared_mime, original_size_bytes, status, client_sha256 
		FROM upload_sessions 
		WHERE id=$1`

	qUpdateSessionCompleted = `
		UPDATE upload_sessions 
		SET status='COMPLETED', client_sha256=$2, updated_at=now() 
		WHERE id=$1`

	qGetSessionForAbort = `
		SELECT temp_blob_key FROM upload_sessions 
		WHERE id=$1 AND user_id=$2`

	qUpdateSessionAborted = `UPDATE upload_sessions SET status='ABORTED', updated_at=now() WHERE id=$1`

	// File content creation
	qInsertFileContent = `
		INSERT INTO file_contents (id, content_hash, blob_key, size_bytes, mime_type, ref_count, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 1, now())
		RETURNING id`

	qInsertUserFile = `
		INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6,'')::uuid, now(), now())
		RETURNING id`

	// Quota checks
	qGetUsedStorage = `
		SELECT COALESCE(SUM(original_size_bytes),0)
		FROM user_files
		WHERE user_id=$1 AND deleted_at IS NULL`

	qGetUserQuota = `SELECT quota_bytes FROM users WHERE id=$1`
)
