package realtime

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"coffeesos/internal/tenant"
)

func TestHubBroadcastsToBrandAndStoreRooms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := NewHub(slog.New(slog.NewTextHandler(nil, nil)), func(*http.Request) bool { return true })

	brand := uuid.New()
	store := uuid.New()
	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		tenant.Set(c, tenant.Principal{UserID: uuid.New(), BrandID: &brand, StoreID: &store, Role: tenant.RoleStaff, Level: tenant.LevelStaff})
	}, hub.ServeWS)
	srv := httptest.NewServer(r)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool { return hub.RoomSize(BrandRoom(brand)) == 1 && hub.RoomSize(StoreRoom(store)) == 1 })

	hub.Broadcast(BrandRoom(brand), "menu.item.availability", map[string]any{"itemId": "x", "isAvailable": false})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ev Event
	if err := json.Unmarshal(msg, &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Type != "menu.item.availability" || ev.Room != BrandRoom(brand) {
		t.Fatalf("unexpected event %+v", ev)
	}

	// Events for another brand must not leak.
	hub.Broadcast(BrandRoom(uuid.New()), "noise", nil)
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("received an event from another brand's room")
	}

	conn.Close()
	waitFor(t, func() bool { return hub.RoomSize(BrandRoom(brand)) == 0 })
}

func TestHubRejectsPrincipalWithoutRoom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := NewHub(slog.New(slog.NewTextHandler(nil, nil)), func(*http.Request) bool { return true })
	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		tenant.Set(c, tenant.Principal{UserID: uuid.New(), Role: tenant.RolePlatformAdmin, Level: tenant.LevelPlatformAdmin})
	}, hub.ServeWS)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
