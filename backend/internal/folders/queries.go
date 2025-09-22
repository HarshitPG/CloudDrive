package folders

const (
	qListRootPrimary = `
SELECT f.id, f.name, f.created_at, f.updated_at,
       COALESCE((SELECT SUM(fc.size_bytes)
                 FROM user_files uf
                 JOIN file_contents fc ON uf.content_id = fc.id
                 WHERE uf.folder_id = f.id AND uf.deleted_at IS NULL), 0) AS size
FROM folders f
WHERE f.user_id = $1 AND f.parent_id IS NULL AND f.deleted_at IS NULL
ORDER BY f.created_at DESC
LIMIT $2 OFFSET $3`

	qListChildrenPrimary = `
SELECT f.id, f.name, f.created_at, f.updated_at,
       COALESCE((SELECT SUM(fc.size_bytes)
                 FROM user_files uf
                 JOIN file_contents fc ON uf.content_id = fc.id
                 WHERE uf.folder_id = f.id AND uf.deleted_at IS NULL), 0) AS size
FROM folders f
JOIN folders p ON f.parent_id = p.id
WHERE f.user_id = $1 AND f.parent_id = $2 AND f.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY f.created_at DESC
LIMIT $3 OFFSET $4`

	qListTrashPrimary = `
SELECT f.id, f.name, f.created_at, f.updated_at, f.deleted_at,
       COALESCE((SELECT SUM(fc.size_bytes)
                 FROM user_files uf
                 JOIN file_contents fc ON uf.content_id = fc.id
                 WHERE uf.folder_id = f.id AND uf.deleted_at IS NULL), 0) AS size
FROM folders f
LEFT JOIN folders p ON f.parent_id = p.id
WHERE f.user_id = $1 AND f.deleted_at IS NOT NULL
  AND (f.parent_id IS NULL OR p.deleted_at IS NULL)
ORDER BY f.deleted_at DESC
LIMIT $2 OFFSET $3`

	qCreateFolder = `
INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
VALUES (gen_random_uuid(), $1, NULLIF($2,'')::uuid, $3, now(), now())
RETURNING id`

	qListSubfolders = `
SELECT id, name, created_at FROM folders
WHERE user_id=$1 AND parent_id=$2 AND deleted_at IS NULL
ORDER BY created_at DESC`

	qListFolderFiles = `
SELECT id, filename, declared_mime, original_size_bytes, created_at
FROM user_files
WHERE user_id=$1 AND folder_id=$2 AND deleted_at IS NULL
ORDER BY created_at DESC`

	qRenameFolder = `UPDATE folders SET name=$1, updated_at=now() WHERE id=$2 AND user_id=$3`

	qMoveFolder = `UPDATE folders SET parent_id=$1, updated_at=now() WHERE id=$2 AND user_id=$3`

	qValidateFolderOwner = `SELECT user_id FROM folders WHERE id=$1 AND deleted_at IS NULL`

	qListFilesInFolder = `
SELECT uf.id, uf.filename, uf.declared_mime, uf.original_size_bytes,
       uf.created_at, uf.updated_at, uf.download_count,
       fc.content_hash, fc.size_bytes, fc.ref_count
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.user_id=$1 AND uf.folder_id=$2 AND uf.deleted_at IS NULL
ORDER BY uf.created_at DESC
LIMIT $3 OFFSET $4`

	qTreeFolder   = `SELECT name, created_at FROM folders WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`
	qTreeChildren = `SELECT id FROM folders WHERE parent_id=$1 AND user_id=$2 AND deleted_at IS NULL`

	qAncestors = `
WITH RECURSIVE anc AS (
    SELECT id, parent_id, name FROM folders WHERE id=$1 AND user_id=$2
    UNION ALL
    SELECT f.id, f.parent_id, f.name FROM folders f
    JOIN anc a ON f.id = a.parent_id
    WHERE f.user_id = $2
)
SELECT id, name FROM anc`

	// Soft delete subtree
	qSoftDeleteSubtreeFiles = `
WITH RECURSIVE subfolders AS (
    SELECT id FROM folders WHERE id=$1 AND user_id=$2
    UNION ALL
    SELECT f.id FROM folders f
    INNER JOIN subfolders sf ON f.parent_id = sf.id
    WHERE f.user_id = $2
)
SELECT uf.id, uf.content_id, fc.blob_key
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.user_id=$2 AND uf.folder_id IN (SELECT id FROM subfolders) AND uf.deleted_at IS NULL`

	qMarkFileDeleted    = `UPDATE user_files SET deleted_at = now() WHERE id=$1`
	qDecrementRef       = `UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1`
	qRevokeFileShare    = `UPDATE shares SET revoked=true WHERE target_type='file' AND target_id=$1`
	qRevokeFolderShares = `
WITH RECURSIVE subfolders AS (
    SELECT id FROM folders WHERE id=$1 AND user_id=$2
    UNION ALL
    SELECT f.id FROM folders f
    INNER JOIN subfolders sf ON f.parent_id = sf.id
    WHERE f.user_id = $2
)
UPDATE shares SET revoked=true WHERE target_type='folder' AND target_id IN (SELECT id FROM subfolders)`

	qMarkFoldersDeleted = `
WITH RECURSIVE subfolders AS (
    SELECT id FROM folders WHERE id=$1 AND user_id=$2
    UNION ALL
    SELECT f.id FROM folders f
    INNER JOIN subfolders sf ON f.parent_id = sf.id
    WHERE f.user_id = $2
)
UPDATE folders SET deleted_at = now() WHERE id IN (SELECT id FROM subfolders)`

	// Hard delete subtree
	qListAllSubtreeFiles = `
WITH RECURSIVE subfolders AS (
    SELECT id FROM folders WHERE id=$1 AND user_id=$2
    UNION ALL
    SELECT f.id FROM folders f
    INNER JOIN subfolders sf ON f.parent_id = sf.id
    WHERE f.user_id = $2
)
SELECT uf.id, uf.content_id, fc.blob_key
FROM user_files uf
JOIN file_contents fc ON uf.content_id = fc.id
WHERE uf.user_id=$2 AND uf.folder_id IN (SELECT id FROM subfolders)`

	qDeleteUserFile     = `DELETE FROM user_files WHERE id=$1`
	qDecRefReturn       = `UPDATE file_contents SET ref_count = GREATEST(ref_count - 1, 0) WHERE id=$1 RETURNING ref_count`
	qCountRefs          = `SELECT COUNT(1) FROM user_files WHERE content_id=$1`
	qDeleteContent      = `DELETE FROM file_contents WHERE id=$1`
	qDeleteFileShares   = `DELETE FROM shares WHERE target_type='file' AND target_id=$1`
	qDeleteFolderShares = `
WITH RECURSIVE subfolders AS (
    SELECT id FROM folders WHERE id=$1 AND user_id=$2
    UNION ALL
    SELECT f.id FROM folders f
    INNER JOIN subfolders sf ON f.parent_id = sf.id
    WHERE f.user_id = $2
)
DELETE FROM shares WHERE target_type='folder' AND target_id IN (SELECT id FROM subfolders)`

	qDeleteFolders = `
WITH RECURSIVE subfolders AS (
    SELECT id FROM folders WHERE id=$1 AND user_id=$2
    UNION ALL
    SELECT f.id FROM folders f
    INNER JOIN subfolders sf ON f.parent_id = sf.id
    WHERE f.user_id = $2
)
DELETE FROM folders WHERE id IN (SELECT id FROM subfolders)`
)
