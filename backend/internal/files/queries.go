package files

const (
	qSelectForDownload = `
SELECT uf.content_id, fc.blob_key, uf.user_id
FROM user_files uf
JOIN file_contents fc ON uf.content_id=fc.id
WHERE uf.id=$1 AND uf.deleted_at IS NULL`

	qCheckShareAccess = `
SELECT COUNT(*) 
FROM shares 
WHERE target_type='file' 
  AND target_id=$1 
  AND revoked=false 
  AND (is_public=true OR shared_with_user_id=$2)`

	qIncDownload = `
UPDATE user_files SET download_count = download_count + 1 WHERE id=$1 RETURNING download_count`

	qSelectOwnerContentFolder = `
SELECT uf.user_id, uf.content_id, fc.blob_key, uf.folder_id
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.id=$1 AND `

	qSoftDeleteFile = `UPDATE user_files SET deleted_at = now() WHERE id=$1`
	qDecRefCount    = `UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1`
	qRevokeShares   = `UPDATE shares SET revoked=true WHERE target_type='file' AND target_id=$1`

	qDeleteUserFile            = `DELETE FROM user_files WHERE id=$1`
	qDecRefReturn              = `UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1 RETURNING ref_count`
	qCountUserFilesByContentID = `SELECT COUNT(*) FROM user_files WHERE content_id=$1`
	qDeleteFileContent         = `DELETE FROM file_contents WHERE id=$1`
	qDeleteSharesPermanent     = `DELETE FROM shares WHERE target_type='file' AND target_id=$1`

	qRestoreFile         = `UPDATE user_files SET deleted_at = NULL WHERE id=$1`
	qIncRefCount         = `UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1`
	qEnableShares        = `UPDATE shares SET revoked=false WHERE target_type='file' AND target_id=$1`
	qSelectFolderForFile = `SELECT folder_id FROM user_files WHERE id=$1`
	qCheckSoftDeleted    = `SELECT user_id, content_id FROM user_files WHERE id=$1 AND deleted_at IS NOT NULL`

	qSelectOwnerActive = `SELECT user_id FROM user_files WHERE id=$1 AND deleted_at IS NULL`
	qUpdateFilename    = `UPDATE user_files SET filename=$1, updated_at=now() WHERE id=$2`
	qUpdateTags        = `UPDATE user_files SET tags=$1, updated_at=now() WHERE id=$2`

	qSelectFolderOwner    = `SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL`
	qSelectOriginalFolder = `SELECT folder_id FROM user_files WHERE id=$1`
	qUpdateMove           = `UPDATE user_files SET folder_id=$1, updated_at=now() WHERE id=$2`

	qSelectOwnerAny = `SELECT user_id FROM user_files WHERE id=$1`
	qInsertVersion  = `
INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, version_of, created_at, updated_at)
VALUES (gen_random_uuid(), $1, $2, $3, null, 0, $4, now(), now())`

	qListVersions = `SELECT id, content_id, filename, created_at FROM user_files WHERE version_of=$1 ORDER BY created_at DESC`

	qGetMetadata = `
SELECT uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       uf.folder_id,
       fc.content_hash, fc.size_bytes, fc.ref_count
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.id=$1 AND uf.user_id=$2 AND uf.deleted_at IS NULL`

	qListTrash = `
SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       fc.content_hash, fc.size_bytes, fc.ref_count,
       uf.deleted_at
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.user_id=$1 AND uf.deleted_at IS NOT NULL
ORDER BY uf.deleted_at DESC`

	qListInFolder = `
SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       fc.content_hash, fc.size_bytes, fc.ref_count
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.user_id=$1 AND uf.folder_id=$2 AND uf.deleted_at IS NULL
ORDER BY uf.created_at DESC`

	qListRoot = `
SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       fc.content_hash, fc.size_bytes, fc.ref_count
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.user_id=$1 AND uf.folder_id IS NULL AND uf.deleted_at IS NULL
ORDER BY uf.created_at DESC`

	qListTrashPrimary = `
SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       fc.content_hash, fc.size_bytes, fc.ref_count,
       uf.deleted_at
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
LEFT JOIN folders f ON uf.folder_id = f.id
WHERE uf.user_id=$1 AND uf.deleted_at IS NOT NULL
  AND (uf.folder_id IS NULL OR f.deleted_at IS NULL)
ORDER BY uf.deleted_at DESC`

	qListInFolderPrimary = `
SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       fc.content_hash, fc.size_bytes, fc.ref_count
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
JOIN folders f ON uf.folder_id = f.id
WHERE uf.user_id=$1 AND uf.folder_id=$2 AND uf.deleted_at IS NULL AND f.deleted_at IS NULL
ORDER BY uf.created_at DESC`

	qListRootPrimary = `
SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       fc.content_hash, fc.size_bytes, fc.ref_count
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
LEFT JOIN folders f ON uf.folder_id = f.id
WHERE uf.user_id=$1 AND uf.folder_id IS NULL AND uf.deleted_at IS NULL
ORDER BY uf.created_at DESC`
)
