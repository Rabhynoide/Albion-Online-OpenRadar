package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/logger"
	"github.com/nospy/albion-openradar/internal/photon"
)

const (
	MaxWebSocketClients = 100
	BatchInterval       = 16 * time.Millisecond // ~60 fps
	MaxBatchSize        = 100
)

// WSBatchMessage represents a batch of messages
type WSBatchMessage struct {
	Type     string        `json:"type"`
	Messages []interface{} `json:"messages"`
}

// WSStats holds WebSocket statistics
type WSStats struct {
	BatchesSent   uint64
	MessagesSent  uint64
	MessagesQueue int
	BytesSent     uint64
}

// WebSocketHandler manages WebSocket connections and broadcasts
type WebSocketHandler struct {
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	upgrader  websocket.Upgrader
	logger    *logger.Logger

	// Batching
	batchBuffer []interface{}
	batchMu     sync.Mutex
	batchTicker *time.Ticker
	stopBatch   chan struct{}

	// Stats
	batchesSent  uint64
	messagesSent uint64
	bytesSent    uint64
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(log *logger.Logger) *WebSocketHandler {
	ws := &WebSocketHandler{
		clients:     make(map[*websocket.Conn]bool),
		logger:      log,
		batchBuffer: make([]interface{}, 0, MaxBatchSize),
		stopBatch:   make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
	ws.startBatchTicker()
	return ws
}

func (ws *WebSocketHandler) startBatchTicker() {
	ws.batchTicker = time.NewTicker(BatchInterval)
	go func() {
		for {
			select {
			case <-ws.batchTicker.C:
				ws.flushBatch()
			case <-ws.stopBatch:
				ws.batchTicker.Stop()
				return
			}
		}
	}()
}

func (ws *WebSocketHandler) flushBatch() {
	ws.batchMu.Lock()
	if len(ws.batchBuffer) == 0 {
		ws.batchMu.Unlock()
		return
	}
	batch := ws.batchBuffer
	ws.batchBuffer = make([]interface{}, 0, MaxBatchSize)
	ws.batchMu.Unlock()

	data, sent := ws.marshalBatch(batch)
	if sent == 0 {
		return
	}
	msgCount := uint64(sent)

	dataLen := uint64(len(data))
	var failedClients []*websocket.Conn
	var sentCount uint64

	ws.clientsMu.RLock()
	for client := range ws.clients {
		if err := client.WriteMessage(websocket.TextMessage, data); err != nil {
			failedClients = append(failedClients, client)
		} else {
			sentCount++
		}
	}
	ws.clientsMu.RUnlock()

	ws.batchesSent++
	ws.messagesSent += msgCount
	ws.bytesSent += dataLen * sentCount

	if len(failedClients) > 0 {
		ws.clientsMu.Lock()
		for _, client := range failedClients {
			if _, exists := ws.clients[client]; exists {
				_ = client.Close()
				delete(ws.clients, client)
			}
		}
		ws.clientsMu.Unlock()
	}
}

// marshalBatch tries the whole batch first (the common, cheap path). Some Photon parameters
// decode to a raw float32 array (see internal/photon), and a bit pattern that happens to be
// NaN/Inf is a real, observed occurrence there - Go's encoding/json refuses to serialize either
// (unlike JS's more permissive JSON.stringify), which fails the whole batch's Marshal call. On
// that failure, this falls back to marshaling messages individually and keeps everything that
// DOES marshal successfully, dropping only the actual offender(s) - previously, one bad message
// out of a batch of (say) 10 meant the other 9 never reached the browser at all.
func (ws *WebSocketHandler) marshalBatch(batch []interface{}) ([]byte, int) {
	if data, err := json.Marshal(&WSBatchMessage{Type: "batch", Messages: batch}); err == nil {
		return data, len(batch)
	}

	kept := make([]interface{}, 0, len(batch))
	for i, m := range batch {
		if _, err := json.Marshal(m); err != nil {
			logger.PrintWarn("WS", "dropping unmarshalable message[%d]: %v (type=%T)", i, err, m)
			continue
		}
		kept = append(kept, m)
	}
	if len(kept) == 0 {
		logger.PrintWarn("WS", "batch marshal failed, entire batch of %d unmarshalable, DROPPED", len(batch))
		return nil, 0
	}

	data, err := json.Marshal(&WSBatchMessage{Type: "batch", Messages: kept})
	if err != nil {
		logger.PrintWarn("WS", "batch marshal failed even after filtering: %v, DROPPED", err)
		return nil, 0
	}
	return data, len(kept)
}

// Stats returns current WebSocket statistics
func (ws *WebSocketHandler) Stats() WSStats {
	ws.batchMu.Lock()
	queueLen := len(ws.batchBuffer)
	ws.batchMu.Unlock()

	return WSStats{
		BatchesSent:   ws.batchesSent,
		MessagesSent:  ws.messagesSent,
		MessagesQueue: queueLen,
		BytesSent:     ws.bytesSent,
	}
}

// ServeHTTP implements http.Handler for WebSocket upgrades
func (ws *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws.handleConnection(w, r)
}

// handleConnection handles new WebSocket connections
func (ws *WebSocketHandler) handleConnection(w http.ResponseWriter, r *http.Request) {
	// Upgrade connection first (doesn't require lock)
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.PrintError("WS", "Upgrade error: %v", err)
		return
	}

	// Check limit AND register atomically to fix race condition
	ws.clientsMu.Lock()
	if len(ws.clients) >= MaxWebSocketClients {
		ws.clientsMu.Unlock()
		_ = conn.Close()
		logger.PrintWarn("WS", "Connection rejected: max clients reached (%d)", MaxWebSocketClients)
		return
	}
	ws.clients[conn] = true
	clientCount := len(ws.clients)
	ws.clientsMu.Unlock()

	logger.PrintInfo("WS", "Client connected (%d total)", clientCount)

	// Handle incoming messages (for logs from client)
	go ws.handleMessages(conn)
}

// handleMessages handles incoming messages from a client
func (ws *WebSocketHandler) handleMessages(conn *websocket.Conn) {
	defer func() {
		ws.clientsMu.Lock()
		delete(ws.clients, conn)
		clientCount := len(ws.clients)
		ws.clientsMu.Unlock()
		_ = conn.Close()
		logger.PrintInfo("WS", "Client disconnected (%d remaining)", clientCount)
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(
				err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
			) {
				logger.PrintError("WS", "Read error: %v", err)
			}
			break
		}

		// Parse incoming message (for logs)
		var data struct {
			Type string        `json:"type"`
			Logs []interface{} `json:"logs"`
		}
		if err := json.Unmarshal(message, &data); err == nil {
			if data.Type == "logs" && len(data.Logs) > 0 && ws.logger != nil {
				ws.logger.WriteLogs(data.Logs)
			}
		}
	}
}

// CloseAllClients closes all WebSocket connections gracefully
func (ws *WebSocketHandler) CloseAllClients() {
	close(ws.stopBatch)
	ws.flushBatch() // Flush remaining events

	ws.clientsMu.Lock()
	for client := range ws.clients {
		_ = client.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
		)
		_ = client.Close()
		delete(ws.clients, client)
	}
	ws.clientsMu.Unlock()
}

// broadcastPayload adds a message to the batch buffer
func (ws *WebSocketHandler) broadcastPayload(code string, payload interface{}) {
	msg := map[string]interface{}{
		"code":       code,
		"dictionary": payload,
	}
	ws.batchMu.Lock()
	ws.batchBuffer = append(ws.batchBuffer, msg)
	ws.batchMu.Unlock()
}

// BroadcastEvent broadcasts an event to all clients
func (ws *WebSocketHandler) BroadcastEvent(event *photon.EventData) {
	ws.broadcastPayload("event", map[string]interface{}{
		"code":       event.Code,
		"parameters": event.Parameters,
	})
}

// BroadcastRequest broadcasts a request to all clients
func (ws *WebSocketHandler) BroadcastRequest(req *photon.OperationRequest) {
	ws.broadcastPayload("request", map[string]interface{}{
		"operationCode": req.OperationCode,
		"parameters":    req.Parameters,
	})
}

// BroadcastResponse broadcasts a response to all clients
func (ws *WebSocketHandler) BroadcastResponse(resp *photon.OperationResponse) {
	ws.broadcastPayload("response", map[string]interface{}{
		"operationCode": resp.OperationCode,
		"returnCode":    resp.ReturnCode,
		"debugMessage":  resp.DebugMessage,
		"parameters":    resp.Parameters,
	})
}

// ClientCount returns the number of connected clients
func (ws *WebSocketHandler) ClientCount() int {
	ws.clientsMu.RLock()
	defer ws.clientsMu.RUnlock()
	return len(ws.clients)
}
