package share

const (
	// Share resolution queries
	qGetShareByToken = `
		SELECT id, target_type, target_id, expires_at
		FROM shares
		WHERE token=$1 AND revoked=false`

	qGetFolderFilesForShare = `
		SELECT uf.id, uf.filename, fc.blob_key, fc.size_bytes
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.folder_id = $1 AND uf.deleted_at IS NULL
		LIMIT 100`

	qGetFileShareDetails = `
		SELECT uf.id, uf.filename, fc.blob_key, fc.size_bytes
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.id=$1 AND uf.deleted_at IS NULL`

	qGetFolderShareDetails = `
		SELECT f.id, f.name, f.created_at, u.full_name
		FROM folders f
		JOIN users u ON f.user_id = u.id
		WHERE f.id = $1 AND f.deleted_at IS NULL`

	// Public share creation
	qGetFileOwner = `SELECT user_id FROM user_files WHERE id=$1 AND deleted_at IS NULL`

	qInsertFileShare = `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'file', $3, $4, $5, $6, now())
		RETURNING id`

	qSetFilePublic = `UPDATE user_files SET is_public = true WHERE id=$1`

	qGetFolderOwner = `SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL`

	qInsertFolderShare = `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'folder', $3, $4, $5, $6, now())
		RETURNING id`

	// User-specific sharing
	qGetTargetUserByEmail = `SELECT id FROM users WHERE email=$1`

	qInsertUserFileShare = `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'file', $3, NULL, NULL, NULL, now())
		RETURNING id`

	qInsertShareUser = `
		INSERT INTO share_users (share_id, target_user_id, permission, created_at)
		VALUES ($1, $2, $3, now())`

	qInsertUserFolderShare = `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'folder', $3, NULL, NULL, NULL, now())
		RETURNING id`

	// List file shares
	qGetPublicFileShare = `
		SELECT id, token, title, description, expires_at, created_at
		FROM shares
		WHERE target_type='file' AND target_id=$1 AND revoked=false
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC LIMIT 1`

	qGetUserFileShares = `
		SELECT su.id, su.target_user_id, su.permission, su.created_at
		FROM share_users su
		JOIN shares s ON su.share_id = s.id
		WHERE s.target_type='file' AND s.target_id=$1`

	// Revoke share
	qGetShareOwnership = `SELECT creator_id, target_type, target_id FROM shares WHERE id=$1`

	qRevokeShare = `UPDATE shares SET revoked=true WHERE id=$1`

	qGetShareToken = `SELECT token FROM shares WHERE id=$1`

	// ListSharedFolderContents authorization and data queries
	qCheckFolderShareAuth = `
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id FROM folders WHERE id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT f.id, f.parent_id FROM folders f
			JOIN ancestors a ON f.id = a.parent_id
		)
		SELECT 1
		FROM shares s
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $2
		  AND s.target_type = 'folder'
		  AND s.revoked = false
		  AND s.target_id IN (SELECT id FROM ancestors)
		LIMIT 1`

	qGetFolderInfo = `SELECT name, parent_id::text FROM folders WHERE id=$1 AND deleted_at IS NULL`

	qListSharedSubfolders = `
		SELECT id, name, created_at
		FROM folders
		WHERE parent_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`

	qListSharedFiles = `
		SELECT uf.id, uf.filename, uf.original_size_bytes, uf.declared_mime, uf.created_at
		FROM user_files uf
		WHERE uf.folder_id = $1 AND uf.deleted_at IS NULL
		ORDER BY uf.created_at DESC`

	// Download shared file queries
	qGetFileDownloadInfo = `
		SELECT COALESCE(uf.folder_id::text, ''), fc.blob_key, uf.filename
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.id = $1 AND uf.deleted_at IS NULL`

	qCheckDirectFileShare = `
		SELECT 1 FROM shares s
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $1
		  AND s.target_type = 'file'
		  AND s.target_id = $2
		  AND s.revoked = false
		LIMIT 1`

	qCheckFolderShareForFile = `
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id FROM folders WHERE id = NULLIF($1, '')::uuid
			UNION ALL
			SELECT f.id, f.parent_id FROM folders f
			JOIN ancestors a ON f.id = a.parent_id
		)
		SELECT 1
		FROM shares s
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $2
		  AND s.target_type = 'folder'
		  AND s.revoked = false
		  AND s.target_id IN (SELECT id FROM ancestors)
		LIMIT 1`

	// List shared folder ancestors (breadcrumb navigation)
	qListFolderAncestors = `
		WITH RECURSIVE anc AS (
			SELECT id, parent_id, name FROM folders WHERE id = $1
			UNION ALL
			SELECT f.id, f.parent_id, f.name FROM folders f JOIN anc a ON f.id = a.parent_id
		)
		SELECT id, name FROM anc`

	// List shared with me queries
	qListSharedFilesWithMe = `
		SELECT DISTINCT ON (uf.id)
			   uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
			   uf.created_at, uf.updated_at, uf.download_count, u.email
		FROM user_files uf
		JOIN users u ON u.id = uf.user_id
		JOIN shares s ON s.target_type='file' AND s.target_id = uf.id AND s.revoked = false
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $1 AND uf.deleted_at IS NULL
		ORDER BY uf.id, uf.created_at DESC
		LIMIT $2 OFFSET $3`

	qListSharedFoldersWithMe = `
		SELECT DISTINCT ON (f.id)
			   f.id, f.name, f.created_at, f.updated_at, u.email,
			   COALESCE((
				   SELECT SUM(fc.size_bytes)
				   FROM user_files uf2
				   JOIN file_contents fc ON uf2.content_id = fc.id
				   WHERE uf2.folder_id = f.id AND uf2.deleted_at IS NULL
			   ), 0) as size
		FROM folders f
		JOIN users u ON u.id = f.user_id
		JOIN shares s ON s.target_type='folder' AND s.target_id = f.id AND s.revoked = false
		JOIN share_users su ON su.share_id = s.id
		WHERE su.target_user_id = $1 AND f.deleted_at IS NULL
		ORDER BY f.id, f.created_at DESC
		LIMIT $2 OFFSET $3`

	// Folder share contents
	qGetFolderShareInfo = `
		SELECT s.id, s.token, s.creator_id, s.target_id, f.name, s.title, s.description,
		       s.recursive, s.snapshot_mode, s.expires_at, s.created_at
		FROM shares s
		JOIN folders f ON s.target_id = f.id
		WHERE s.token=$1 AND s.target_type='folder' AND s.revoked=false`

	qListFolderSubfolders = `
		SELECT id, name, parent_id, created_at
		FROM folders
		WHERE parent_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY name`

	qListFolderFiles = `
		SELECT uf.id, uf.filename, uf.original_size_bytes, uf.declared_mime, uf.folder_id, uf.created_at
		FROM user_files uf
		WHERE uf.folder_id = $1 AND uf.user_id = $2 AND uf.deleted_at IS NULL
		ORDER BY uf.filename`

	// Recursive folder queries
	qListRecursiveFolders = `
		WITH RECURSIVE folder_tree AS (
		    SELECT id, name, parent_id, name as folder_path
		    FROM folders
		    WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		    
		    UNION ALL
		    
		    SELECT f.id, f.name, f.parent_id, 
		           CASE 
		               WHEN ft.folder_path = '' THEN f.name
		               ELSE ft.folder_path || '/' || f.name
		           END as folder_path
		    FROM folders f
		    INNER JOIN folder_tree ft ON f.parent_id = ft.id
		    WHERE f.user_id = $2 AND f.deleted_at IS NULL
		)
		SELECT id, name, parent_id, folder_path
		FROM folder_tree
		WHERE id != $1
		ORDER BY folder_path
		LIMIT $3`

	qListRecursiveFiles = `
		WITH RECURSIVE folder_tree AS (
		    SELECT id, name, parent_id, name as folder_path
		    FROM folders
		    WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		    
		    UNION ALL
		    
		    SELECT f.id, f.name, f.parent_id,
		           CASE 
		               WHEN ft.folder_path = '' THEN f.name
		               ELSE ft.folder_path || '/' || f.name
		           END as folder_path
		    FROM folders f
		    INNER JOIN folder_tree ft ON f.parent_id = ft.id
		    WHERE f.user_id = $2 AND f.deleted_at IS NULL
		)
		SELECT uf.id, uf.filename, uf.original_size_bytes, uf.declared_mime, 
		       uf.folder_id, COALESCE(ft.folder_path, '') as folder_path
		FROM user_files uf
		JOIN folder_tree ft ON uf.folder_id = ft.id
		WHERE uf.user_id = $2 AND uf.deleted_at IS NULL
		ORDER BY folder_path, uf.filename
		LIMIT $3`

	// Shared file download
	qGetSharedFileBlobKey = `
		SELECT fc.blob_key
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.id = $1 AND uf.deleted_at IS NULL`

	// Folder ancestors
	qGetFolderAncestors = `
		WITH RECURSIVE ancestors AS (
		    SELECT id, name, parent_id, 1 AS level
		    FROM folders
		    WHERE id = $1 AND deleted_at IS NULL
		    
		    UNION ALL
		    
		    SELECT f.id, f.name, f.parent_id, a.level + 1
		    FROM folders f
		    INNER JOIN ancestors a ON f.id = a.parent_id
		    WHERE f.deleted_at IS NULL
		)
		SELECT id, name
		FROM ancestors
		WHERE id != $1
		ORDER BY level DESC`

	// Shared with me
	qGetSharedWithMe = `
		SELECT s.id, s.token, s.target_type, s.target_id, s.created_at,
		       su.permission, u.full_name AS shared_by
		FROM shares s
		JOIN share_users su ON s.id = su.share_id
		JOIN users u ON s.creator_id = u.id
		WHERE su.target_user_id = $1 AND s.revoked = false
		  AND (s.expires_at IS NULL OR s.expires_at > now())
		ORDER BY s.created_at DESC`

	qGetSharedFileDetails = `
		SELECT uf.filename, uf.declared_mime, uf.original_size_bytes, uf.created_at
		FROM user_files uf
		WHERE uf.id = $1 AND uf.deleted_at IS NULL`

	qGetSharedFolderDetails = `
		SELECT f.name, f.created_at
		FROM folders f
		WHERE f.id = $1 AND f.deleted_at IS NULL`

	// Folder share creation
	qValidateFolderOwnership = `SELECT user_id, name FROM folders WHERE id=$1 AND deleted_at IS NULL`

	qInsertFolderShareWithOptions = `
		INSERT INTO shares (id, token, creator_id, target_type, target_id, title, description, 
		    expires_at, recursive, snapshot_mode, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'folder', $3, $4, $5, $6, $7, $8, now())
		RETURNING id, created_at`

	qGetShareInfoByID = `
		SELECT s.id, s.token, s.creator_id, s.target_id, f.name, s.title, s.description,
		       s.recursive, s.snapshot_mode, s.expires_at, s.created_at
		FROM shares s
		JOIN folders f ON s.target_id = f.id
		WHERE s.id=$1 AND s.target_type='folder' AND s.revoked=false`

	// Folder share helper queries (foldershare_service.go)
	qRevokeShareByID = `UPDATE shares SET revoked=true WHERE id=$1`

	// Folder share listing queries
	qGetSubfolders = `
		SELECT id, name, parent_id
		FROM folders
		WHERE parent_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY name
		LIMIT $3`

	qGetFolderFiles = `
		SELECT id, filename, original_size_bytes, declared_mime, folder_id
		FROM user_files
		WHERE folder_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY filename
		LIMIT $3`

	qGetRecursiveFolders = `
		WITH RECURSIVE folder_tree AS (
		    SELECT id, name, parent_id, name as folder_path
		    FROM folders
		    WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		    
		    UNION ALL
		    
		    SELECT f.id, f.name, f.parent_id, 
		           CASE 
		               WHEN ft.folder_path = '' THEN f.name
		               ELSE ft.folder_path || '/' || f.name
		           END as folder_path
		    FROM folders f
		    INNER JOIN folder_tree ft ON f.parent_id = ft.id
		    WHERE f.user_id = $2 AND f.deleted_at IS NULL
		)
		SELECT id, name, parent_id, folder_path
		FROM folder_tree
		WHERE id != $1
		ORDER BY folder_path
		LIMIT $3`

	qGetRecursiveFiles = `
		WITH RECURSIVE folder_tree AS (
		    SELECT id, name, parent_id, name as folder_path
		    FROM folders
		    WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		    
		    UNION ALL
		    
		    SELECT f.id, f.name, f.parent_id,
		           CASE 
		               WHEN ft.folder_path = '' THEN f.name
		               ELSE ft.folder_path || '/' || f.name
		           END as folder_path
		    FROM folders f
		    INNER JOIN folder_tree ft ON f.parent_id = ft.id
		    WHERE f.user_id = $2 AND f.deleted_at IS NULL
		)
		SELECT uf.id, uf.filename, uf.original_size_bytes, uf.declared_mime, 
		       uf.folder_id, COALESCE(ft.folder_path, '') as folder_path
		FROM user_files uf
		JOIN folder_tree ft ON uf.folder_id = ft.id
		WHERE uf.user_id = $2 AND uf.deleted_at IS NULL
		ORDER BY folder_path, uf.filename
		LIMIT $3`

	// Folder share resolution queries
	qResolveFolderShareByToken = `
		SELECT id, target_id, title, description, recursive, snapshot_mode, expires_at, created_at
		FROM shares
		WHERE token=$1 AND target_type='folder' AND revoked=false`

	qGetFolderName = `SELECT name FROM folders WHERE id=$1`

	qGetFolderOwnerID = `SELECT user_id FROM folders WHERE id = $1 AND deleted_at IS NULL`

	qResolveFolderShareContents = `
		SELECT id, target_id, recursive, expires_at
		FROM shares
		WHERE token=$1 AND target_type='folder' AND revoked=false`

	qGetParentFolder = `SELECT parent_id FROM folders WHERE id=$1 AND deleted_at IS NULL`

	qCheckFolderInTree = `
		WITH RECURSIVE folder_tree AS (
		    SELECT id FROM folders WHERE id = $1 AND deleted_at IS NULL
		    UNION ALL
		    SELECT f.id FROM folders f
		    INNER JOIN folder_tree ft ON f.parent_id = ft.id
		    WHERE f.deleted_at IS NULL
		)
		SELECT COUNT(*) FROM folder_tree WHERE id = $2`

	// Download from share queries
	qGetFileFromShare = `
		SELECT uf.filename, fc.blob_key, fc.size_bytes, uf.folder_id
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.id = $1 AND uf.deleted_at IS NULL`

	qGetFolderShareForDownload = `
		SELECT s.id, s.target_id, s.recursive, s.expires_at, f.name
		FROM shares s
		JOIN folders f ON s.target_id = f.id
		WHERE s.token=$1 AND s.target_type='folder' AND s.revoked=false`

	qGetFileBlobKey = `
		SELECT fc.blob_key
		FROM user_files uf
		JOIN file_contents fc ON uf.content_id = fc.id
		WHERE uf.id = $1 AND uf.deleted_at IS NULL`

	// Check if share requires user authentication
	qCheckShareAuthRequired = `
		SELECT 
			CASE 
				WHEN EXISTS (SELECT 1 FROM share_users WHERE share_id = $1) 
				THEN true 
				ELSE false 
			END as auth_required`

	// Check if user has access to share through share_users
	qCheckUserShareAccess = `
		SELECT 1 
		FROM share_users 
		WHERE share_id = $1 AND target_user_id = $2
		LIMIT 1`
)
