package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	TTLSearch       = 30 * time.Second
	TTLFileMetadata = 10 * time.Minute
	TTLFolderList   = 30 * time.Second
	TTLShareResolve = 2 * time.Minute
	TTLQuotaUsage   = 30 * time.Second
	TTLNegative     = 5 * time.Second
)

const (
	vFile   = "v1"
	vFolder = "v1"
	vShare  = "v1"
	vSearch = "v1"
)

func FileMetadataKey(fileID string) string {
	return fmt.Sprintf("cache:file:%s:%s:metadata", vFile, fileID)
}

func FolderContentsKey(folderID string) string {
	return fmt.Sprintf("cache:folder:%s:%s:contents", vFolder, folderID)
}

func ShareResolveKey(token string) string {
	return fmt.Sprintf("cache:share:%s:%s:resolve", vShare, token)
}

func SearchKey(userID string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	sb := strings.Builder{}
	for _, k := range keys {
		sb.WriteString(k + "=" + url.QueryEscape(params[k]) + "&")
	}
	hash := sha256.Sum256([]byte(sb.String()))
	return "cache:search:" + vSearch + ":" + userID + ":" + hex.EncodeToString(hash[:])
}
