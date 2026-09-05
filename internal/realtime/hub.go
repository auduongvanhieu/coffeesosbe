// Package realtime is the WebSocket fan-out used to push events (new orders,
// sold-out items, status changes) to POS terminals, barista screens and the
// customer app. Clients join a brand room and, if they belong to a store, a
// store room. Producers call Hub.Broadcast; they never touch sockets.
package realtime

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"coffeesos/internal/tenant"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	sendBuffer = 32
)

func BrandRoom(id uuid.UUID) string { return "brand:" + id.String() }
func StoreRoom(id uuid.UUID) string { return "store:" + id.String() }

type Event struct {
	Type string    `json:"type"`
	Room string    `json:"room"`
	At   time.Time `json:"at"`
	Data any       `json:"data,omitempty"`
}

type client struct {
	conn  *websocket.Conn
	send  chan []byte
	rooms []string
}

type Hub struct {
	log      *slog.Logger
	upgrader websocket.Upgrader

	mu    sync.RWMutex
	rooms map[string]map[*client]struct{}
}

// NewHub creates a hub. checkOrigin decides which browser origins may connect.
func NewHub(log *slog.Logger, checkOrigin func(r *http.Request) bool) *Hub {
	return &Hub{
		log:   log,
		rooms: map[string]map[*client]struct{}{},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 4096,
			CheckOrigin:     checkOrigin,
		},
	}
}

// Broadcast sends an event to every client in the room. Slow clients are dropped.
func (h *Hub) Broadcast(room, eventType string, data any) {
	payload, err := json.Marshal(Event{Type: eventType, Room: room, At: time.Now(), Data: data})
	if err != nil {
		h.log.Error("realtime: marshal event", "err", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for cl := range h.rooms[room] {
		select {
		case cl.send <- payload:
		default:
			h.log.Warn("realtime: dropping slow client", "room", room)
			close(cl.send)
			delete(h.rooms[room], cl)
		}
	}
}

// RoomSize is handy for health/debug endpoints and tests.
func (h *Hub) RoomSize(room string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[room])
}

// ServeWS upgrades the connection. Must run behind auth.RequireAuth.
func (h *Hub) ServeWS(c *gin.Context) {
	p := tenant.MustFrom(c)
	var rooms []string
	if p.BrandID != nil {
		rooms = append(rooms, BrandRoom(*p.BrandID))
	}
	if p.StoreID != nil {
		rooms = append(rooms, StoreRoom(*p.StoreID))
	}
	if len(rooms) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "no_room", "message": "platform admins must pass X-Brand-ID"}})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Warn("realtime: upgrade failed", "err", err)
		return
	}
	cl := &client{conn: conn, send: make(chan []byte, sendBuffer), rooms: rooms}
	h.join(cl)

	go h.writePump(cl)
	h.readPump(cl)
}

func (h *Hub) join(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range cl.rooms {
		if h.rooms[r] == nil {
			h.rooms[r] = map[*client]struct{}{}
		}
		h.rooms[r][cl] = struct{}{}
	}
}

func (h *Hub) leave(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range cl.rooms {
		if set, ok := h.rooms[r]; ok {
			if _, present := set[cl]; present {
				delete(set, cl)
				close(cl.send)
			}
			if len(set) == 0 {
				delete(h.rooms, r)
			}
		}
	}
}

// readPump keeps the connection alive and detects closure. Clients only listen
// for now; inbound messages are ignored.
func (h *Hub) readPump(cl *client) {
	defer func() {
		h.leave(cl)
		_ = cl.conn.Close()
	}()
	cl.conn.SetReadLimit(4096)
	_ = cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	cl.conn.SetPongHandler(func(string) error {
		return cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := cl.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) writePump(cl *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = cl.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-cl.send:
			_ = cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = cl.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := cl.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cl.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
