package rest

// This file contains docs-only stubs to ensure swag can resolve @Router paths in this package.

// FilesListDoc godoc
//	@Summary	List files
//	@Tags		files
//	@Produce	json
//	@Param		folderId	query		string	false	"Filter by folder"
//	@Param		deleted		query		boolean	false	"List deleted/trash"
//	@Success	200			{object}	map[string]interface{}
//	@Security	BearerAuth
//	@Router		/api/v1/files [get]
func FilesListDoc() {}

// FoldersListDoc godoc
//	@Summary	List folders
//	@Tags		folders
//	@Produce	json
//	@Param		parentId	query	string	false	"Parent folder ID"
//	@Param		deleted		query	boolean	false	"List deleted folders"
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]interface{}
//	@Router		/api/v1/folders [get]
func FoldersListDoc() {}

// SearchFilesDoc godoc
//	@Summary	Search files
//	@Tags		search
//	@Produce	json
//	@Param		q			query	string	false	"Query"
//	@Param		mime		query	string	false	"MIME type"
//	@Param		folderId	query	string	false	"Folder ID"
//	@Param		uploader	query	string	false	"Uploader user ID"
//	@Param		sort		query	string	false	"Sort order"
//	@Param		limit		query	integer	false	"Limit"
//	@Param		offset		query	integer	false	"Offset"
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]interface{}
//	@Router		/api/v1/search/files [get]
func SearchFilesDoc() {}
