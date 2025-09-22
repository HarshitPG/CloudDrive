package server

// This file exists only to attach Swagger annotations to non-REST style endpoints.

// GraphQLRequest represents a typical GraphQL HTTP request body
type GraphQLRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a typical GraphQL HTTP response body
type GraphQLResponse struct {
	Data   interface{}              `json:"data,omitempty"`
	Errors []map[string]interface{} `json:"errors,omitempty"`
}

// GraphQLDoc godoc
//	@Summary		GraphQL endpoint
//	@Description	Send GraphQL queries and mutations
//	@Tags			graphql
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		GraphQLRequest	true	"GraphQL request"
//	@Success		200		{object}	GraphQLResponse
//	@Router			/api/v1/graphql [post]
func GraphQLDoc() {}

// WSMessage outbound WS message schema
type WSMessage struct {
	FileID        string `json:"fileId"`
	DownloadCount int64  `json:"downloadCount"`
	Timestamp     int64  `json:"ts,omitempty"`
}

// WSDoc godoc
//	@Summary		WebSocket for download updates
//	@Description	Connect via WebSocket to receive download count updates. Optional query param fileId to filter.
//	@Tags			websocket
//	@Param			fileId	query		string		false	"Filter by file ID"
//	@Success		101		{string}	string		"Switching Protocols"
//	@Success		200		{object}	WSMessage	"Example server message (for documentation only)"
//	@Router			/api/v1/ws/downloads [get]
func WSDoc() {}
