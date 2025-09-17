package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"backend/pkg/logger"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// client connection settings
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
	sendBufSize    = 64
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: tighten origins in prod
		return true
	},
}

// Client represents a single websocket connection.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan BroadcastMessage
	// subscription filter: optionally allow clients to restrict to a file id
	// empty => receive all broadcasts
	subFileID string
	ctx       context.Context
}

func (c *Client) close() {
	_ = c.conn.Close()
	c.hub.Unregister(c)
	select {
	case <-c.ctx.Done():
		// already closed
	default:
		// cancel if context has cancel function (not stored here)
	}
}

// writePump pumps messages from send channel to the websocket
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// hub closed channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// if client filters by file id, drop others
			if c.subFileID != "" && c.subFileID != msg.FileID {
				continue
			}

			b, _ := json.Marshal(msg)
			if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
				logger.L.Warn("ws write failed", zap.Error(err))
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// readPump reads client messages (we expect none; used to detect close)
func (c *Client) readPump() {
	defer c.close()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.L.Warn("ws unexpected close", zap.Error(err))
			}
			return
		}
		// We ignore incoming payloads — only support server push
	}
}

// ServeWS upgrades request to websocket and registers client with hub.
// Query params:
// - fileId (optional) => only receive updates for that file
func ServeWS(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// Upgrade
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.L.Warn("ws upgrade failed", zap.Error(err))
			return
		}

		// Use a background context for client lifetime: request context is
		// canceled when the HTTP handler returns which would immediately
		// cancel the client's pumps and close the connection with a 1006.
		// The client will be closed explicitly by its pumps or hub logic.
		ctx := context.Background()
		// subscription filter
		q := r.URL.Query()
		fileID := q.Get("fileId")

		client := &Client{
			hub:       hub,
			conn:      conn,
			send:      make(chan BroadcastMessage, sendBufSize),
			subFileID: fileID,
			ctx:       ctx,
		}
		hub.Register(client)

		go client.writePump()
		go client.readPump()
	}
}
