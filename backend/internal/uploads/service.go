package uploads

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"

	"backend/internal/audit"
	iCache "backend/internal/cache"
	"backend/internal/utils"
	"backend/pkg/logger"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"
)

func (s *service) FolderInit(ctx context.Context, userID, parentID, rootName string, files []FolderInitFile) (FolderInitResponse, error) {
	if userID == "" {
		return FolderInitResponse{}, ErrUnauthorized
	}
	if len(files) == 0 {
		return FolderInitResponse{}, fmt.Errorf("no files provided")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FolderInitResponse{}, err
	}
	defer tx.Rollback()

	var rootFolderID string
	q := `SELECT id FROM folders WHERE user_id=$1 AND COALESCE(parent_id::text,'') = NULLIF($2,'') AND name=$3 AND deleted_at IS NULL LIMIT 1`
	err = tx.QueryRowContext(ctx, q, userID, parentID, rootName).Scan(&rootFolderID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `
            INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at)
            VALUES (gen_random_uuid(), $1, NULLIF($2,'')::uuid, $3, now(), now())
            RETURNING id
        `, userID, parentID, rootName).Scan(&rootFolderID)
		if err != nil {
			return FolderInitResponse{}, err
		}
	} else if err != nil {
		return FolderInitResponse{}, err
	}

	folderMap := map[string]string{"": rootFolderID}

	ensureFolder := func(path string) (string, error) {
		if id, ok := folderMap[path]; ok {
			return id, nil
		}
		for i, ch := range path {
			if ch == '/' {
				seg := path[:i]
				if seg != "" {
					if _, ok := folderMap[seg]; !ok {
						lastSlash := -1
						for j := len(seg) - 1; j >= 0; j-- {
							if seg[j] == '/' {
								lastSlash = j
								break
							}
						}
						var pp, name string
						if lastSlash >= 0 {
							pp = seg[:lastSlash]
							name = seg[lastSlash+1:]
						} else {
							pp = ""
							name = seg
						}
						pid := folderMap[pp]
						var fid string
						if err := tx.QueryRowContext(ctx, `SELECT id FROM folders WHERE user_id=$1 AND parent_id=$2 AND name=$3 AND deleted_at IS NULL LIMIT 1`, userID, pid, name).Scan(&fid); err == sql.ErrNoRows {
							if err := tx.QueryRowContext(ctx, `INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at) VALUES (gen_random_uuid(), $1, $2, $3, now(), now()) RETURNING id`, userID, pid, name).Scan(&fid); err != nil {
								return "", err
							}
						} else if err != nil {
							return "", err
						}
						folderMap[seg] = fid
					}
				}
			}
		}
		if path != "" {
			if _, ok := folderMap[path]; !ok {
				lastSlash := -1
				for j := len(path) - 1; j >= 0; j-- {
					if path[j] == '/' {
						lastSlash = j
						break
					}
				}
				var pp, name string
				if lastSlash >= 0 {
					pp = path[:lastSlash]
					name = path[lastSlash+1:]
				} else {
					pp = ""
					name = path
				}
				pid := folderMap[pp]
				var fid string
				if err := tx.QueryRowContext(ctx, `SELECT id FROM folders WHERE user_id=$1 AND parent_id=$2 AND name=$3 AND deleted_at IS NULL LIMIT 1`, userID, pid, name).Scan(&fid); err == sql.ErrNoRows {
					if err := tx.QueryRowContext(ctx, `INSERT INTO folders (id, user_id, parent_id, name, created_at, updated_at) VALUES (gen_random_uuid(), $1, $2, $3, now(), now()) RETURNING id`, userID, pid, name).Scan(&fid); err != nil {
						return "", err
					}
				} else if err != nil {
					return "", err
				}
				folderMap[path] = fid
			}
		}
		return folderMap[path], nil
	}

	dirSet := map[string]struct{}{}
	for _, f := range files {
		p := f.Path
		last := -1
		for i := len(p) - 1; i >= 0; i-- {
			if p[i] == '/' {
				last = i
				break
			}
		}
		dir := ""
		if last > 0 {
			dir = p[:last]
		}
		if dir != "" {
			dirSet[dir] = struct{}{}
		}
	}
	for d := range dirSet {
		if _, err := ensureFolder(d); err != nil {
			return FolderInitResponse{}, err
		}
	}

	responses := make([]FolderInitFileResponse, 0, len(files))
	foldersOut := []map[string]string{}
	for path, id := range folderMap {
		foldersOut = append(foldersOut, map[string]string{"path": path, "folderId": id})
	}

	for _, f := range files {
		last := -1
		for i := len(f.Path) - 1; i >= 0; i-- {
			if f.Path[i] == '/' {
				last = i
				break
			}
		}
		dir := ""
		name := f.Path
		if last >= 0 {
			dir = f.Path[:last]
			name = f.Path[last+1:]
		}
		targetFID := rootFolderID
		if dir != "" {
			if id, ok := folderMap[dir]; ok {
				targetFID = id
			}
		}

		if f.SHA256 != "" {
			var contentID string
			var sizeBytes int64
			err := tx.QueryRowContext(ctx, "SELECT id, size_bytes FROM file_contents WHERE content_hash=$1 LIMIT 1", f.SHA256).Scan(&contentID, &sizeBytes)
			if err == nil {
				var newUserFileID string
				err = tx.QueryRowContext(ctx, `
                    INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
                    VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, now(), now())
                    RETURNING id
                `, userID, contentID, name, f.Mime, f.Size, targetFID).Scan(&newUserFileID)
				if err != nil {
					return FolderInitResponse{}, err
				}
				if _, err := tx.ExecContext(ctx, "UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
					return FolderInitResponse{}, err
				}
				responses = append(responses, FolderInitFileResponse{Path: f.Path, Deduped: true, UserFileID: newUserFileID})
				continue
			}
		}

		sessionID := uuid.NewString()
		tempName := fmt.Sprintf("tmp/%s/%s", sessionID, name)
		url, err := s.storage.PresignedPutURL(ctx, tempName, 30)
		if err != nil {
			return FolderInitResponse{}, err
		}
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO upload_sessions (id, user_id, filename, declared_mime, original_size_bytes, temp_blob_key, client_sha256, status, created_at, updated_at)
            VALUES ($1,$2,$3,$4,$5,$6,$7,'OPEN',now(),now())
        `, sessionID, userID, name, f.Mime, f.Size, tempName, f.SHA256); err != nil {
			return FolderInitResponse{}, err
		}
		responses = append(responses, FolderInitFileResponse{Path: f.Path, Deduped: false, SessionID: sessionID, UploadUrl: url, TempBlobKey: tempName, TargetFolderID: targetFID})
	}

	if err := tx.Commit(); err != nil {
		return FolderInitResponse{}, err
	}
	return FolderInitResponse{UploadID: uuid.NewString(), RootFolderID: rootFolderID, Folders: foldersOut, Files: responses}, nil
}

func (s *service) CreateSession(ctx context.Context, userID string, req CreateSessionRequest) (CreateSessionResponse, error) {
	if userID == "" {
		return CreateSessionResponse{}, ErrUnauthorized
	}

	if req.OriginalSize > 0 {
		used, quota, err := s.getUsageAndQuota(ctx, userID)
		if err != nil {
			return CreateSessionResponse{}, err
		}
		if used+req.OriginalSize > quota {
			return CreateSessionResponse{}, ErrQuotaExceeded
		}
	}

	if req.ClientSha256 != "" {
		var contentID string
		var sizeBytes int64
		err := s.db.QueryRowContext(ctx, "SELECT id, size_bytes FROM file_contents WHERE content_hash=$1 LIMIT 1", req.ClientSha256).Scan(&contentID, &sizeBytes)
		if err == nil {
			tx, txErr := s.db.BeginTx(ctx, nil)
			if txErr != nil {
				return CreateSessionResponse{}, txErr
			}
			defer tx.Rollback()
			var newUserFileID string
			err = tx.QueryRowContext(ctx, `
                INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, created_at, updated_at)
                VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
                RETURNING id
            `, userID, contentID, req.Filename, req.DeclaredMime, req.OriginalSize).Scan(&newUserFileID)
			if err != nil {
				return CreateSessionResponse{}, err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
				return CreateSessionResponse{}, err
			}
			if err := tx.Commit(); err != nil {
				return CreateSessionResponse{}, err
			}
			return CreateSessionResponse{SkipUpload: true, ExistingFileId: newUserFileID, UserFileId: newUserFileID}, nil
		}
	}

	sessionID := uuid.NewString()
	tempName := fmt.Sprintf("tmp/%s/%s", sessionID, req.Filename)
	url, err := s.storage.PresignedPutURL(ctx, tempName, 30)
	if err != nil {
		return CreateSessionResponse{}, err
	}
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO upload_sessions (id, user_id, filename, declared_mime, original_size_bytes, temp_blob_key, client_sha256, status, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,'OPEN',now(),now())
    `, sessionID, userID, req.Filename, req.DeclaredMime, req.OriginalSize, tempName, req.ClientSha256)
	if err != nil {
		return CreateSessionResponse{}, err
	}
	return CreateSessionResponse{SessionId: sessionID, UploadUrl: url, TempBlobKey: tempName, SkipUpload: false}, nil
}

func (s *service) Complete(ctx context.Context, userID string, req CompleteRequest) (CompleteResponse, error) {
	if userID == "" {
		return CompleteResponse{}, ErrUnauthorized
	}

	var tempKey, filename, declaredMime, clientSha, status string
	var originalSize int64
	err := s.db.QueryRowContext(ctx, "SELECT temp_blob_key, filename, declared_mime, original_size_bytes, status, client_sha256 FROM upload_sessions WHERE id=$1", req.SessionId).Scan(&tempKey, &filename, &declaredMime, &originalSize, &status, &clientSha)
	if err != nil {
		if err == sql.ErrNoRows {
			return CompleteResponse{}, ErrNotFound
		}
		return CompleteResponse{}, err
	}
	if status != "OPEN" {
		return CompleteResponse{}, fmt.Errorf("session not open")
	}

	sha := req.ClientSha256
	if sha == "" && clientSha != "" {
		sha = clientSha
	}

	objReader, err := s.storage.GetObjectReader(ctx, tempKey)
	if err != nil {
		return CompleteResponse{}, fmt.Errorf("uploaded object not found")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(objReader, head)

	if sha == "" {
		contentReader := io.MultiReader(bytes.NewReader(head[:n]), objReader)
		computed, err := utils.ComputeSHA256(contentReader)
		if err != nil {
			_ = objReader.Close()
			return CompleteResponse{}, err
		}
		sha = computed
	}
	_ = objReader.Close()

	sniffedMime := http.DetectContentType(head[:n])
	if declaredMime != "" && declaredMime != sniffedMime {
		ext := filepath.Ext(filename)
		alias := mime.TypeByExtension(ext)
		if !(alias == sniffedMime || alias == declaredMime) {
			return CompleteResponse{}, fmt.Errorf("declared mime mismatch")
		}
	}
	if declaredMime == "" {
		declaredMime = sniffedMime
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompleteResponse{}, err
	}
	defer tx.Rollback()

	var contentID string
	err = tx.QueryRowContext(ctx, "SELECT id FROM file_contents WHERE content_hash=$1 LIMIT 1", sha).Scan(&contentID)
	if err == nil {
		var newUserFileID string
		err = tx.QueryRowContext(ctx, `
            INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
            VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6,'')::uuid, now(), now())
            RETURNING id
        `, userID, contentID, filename, declaredMime, originalSize, req.FolderID).Scan(&newUserFileID)
		if err != nil {
			return CompleteResponse{}, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", contentID); err != nil {
			return CompleteResponse{}, err
		}
		_, _ = tx.ExecContext(ctx, "UPDATE upload_sessions SET status='COMPLETED', client_sha256=$2, updated_at=now() WHERE id=$1", req.SessionId, sha)
		if err := tx.Commit(); err != nil {
			return CompleteResponse{}, err
		}
		if s.cache != nil && req.FolderID != "" {
			iCache.InvalidateFolder(ctx, s.cache, req.FolderID)
		}
		return CompleteResponse{UserFileID: newUserFileID, Deduped: true}, nil
	}

	used, quota, errQ := s.getUsageAndQuota(ctx, userID)
	if errQ != nil {
		return CompleteResponse{}, errQ
	}
	if used+originalSize > quota {
		return CompleteResponse{}, ErrQuotaExceeded
	}

	finalKey := fmt.Sprintf("objects/%s", sha)
	if err := s.storage.CopyTempToObject(ctx, tempKey, finalKey); err != nil {
		return CompleteResponse{}, err
	}

	var newContentID string
	err = tx.QueryRowContext(ctx, `
        INSERT INTO file_contents (id, content_hash, blob_key, size_bytes, mime_type, ref_count, created_at)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, 1, now())
        RETURNING id
    `, sha, finalKey, originalSize, declaredMime).Scan(&newContentID)
	if err != nil {
		logger.L.Warn("insert file_contents failed, fallback to select", zap.Error(err))
		err2 := tx.QueryRowContext(ctx, "SELECT id FROM file_contents WHERE content_hash=$1 LIMIT 1", sha).Scan(&newContentID)
		if err2 != nil {
			return CompleteResponse{}, err2
		}
		if _, err := tx.ExecContext(ctx, "UPDATE file_contents SET ref_count = ref_count + 1 WHERE id=$1", newContentID); err != nil {
			return CompleteResponse{}, err
		}
	}

	var newUserFileID string
	err = tx.QueryRowContext(ctx, `
        INSERT INTO user_files (id, user_id, content_id, filename, declared_mime, original_size_bytes, folder_id, created_at, updated_at)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6,'')::uuid, now(), now())
        RETURNING id
    `, userID, newContentID, filename, declaredMime, originalSize, req.FolderID).Scan(&newUserFileID)
	if err != nil {
		return CompleteResponse{}, err
	}

	_, _ = tx.ExecContext(ctx, "UPDATE upload_sessions SET status='COMPLETED', client_sha256=$2, updated_at=now() WHERE id=$1", req.SessionId, sha)
	_ = audit.Log(ctx, s.db, userID, "upload", "file", newUserFileID, map[string]interface{}{"filename": filename, "size": originalSize, "sha256": sha})

	if err := tx.Commit(); err != nil {
		return CompleteResponse{}, err
	}
	if s.cache != nil && req.FolderID != "" {
		iCache.InvalidateFolder(ctx, s.cache, req.FolderID)
	}
	return CompleteResponse{UserFileID: newUserFileID, ContentID: newContentID, Deduped: false}, nil
}

func (s *service) Abort(ctx context.Context, userID, sessionID string) error {
	if userID == "" {
		return ErrUnauthorized
	}
	var tempKey string
	err := s.db.QueryRowContext(ctx, "SELECT temp_blob_key FROM upload_sessions WHERE id=$1 AND user_id=$2", sessionID, userID).Scan(&tempKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	err = s.storage.Client.RemoveObject(ctx, s.storage.Bucket, tempKey, minio.RemoveObjectOptions{})
	if err != nil {
		logger.L.Warn("remove temp object failed", zap.Error(err))
	}
	_, _ = s.db.ExecContext(ctx, "UPDATE upload_sessions SET status='ABORTED', updated_at=now() WHERE id=$1", sessionID)
	return nil
}

func (s *service) getUsageAndQuota(ctx context.Context, userID string) (int64, int64, error) {
	var used int64
	if err := s.db.QueryRowContext(ctx, `
        SELECT COALESCE(SUM(original_size_bytes),0)
        FROM user_files
        WHERE user_id=$1 AND deleted_at IS NULL
    `, userID).Scan(&used); err != nil {
		return 0, 0, err
	}
	var quota int64
	if err := s.db.QueryRowContext(ctx, "SELECT quota_bytes FROM users WHERE id=$1", userID).Scan(&quota); err != nil {
		return 0, 0, err
	}
	return used, quota, nil
}
