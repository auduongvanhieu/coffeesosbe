package order

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"coffeesos/internal/db"
	"coffeesos/internal/httpx"
	"coffeesos/internal/tenant"
)

type Handler struct {
	svc *Service
	q   *db.Queries
}

func NewHandler(svc *Service, q *db.Queries) *Handler {
	return &Handler{svc: svc, q: q}
}

// fail maps service errors to HTTP.
func fail(c *gin.Context, err error) {
	var inv *InvalidOrderError
	var st *StateError
	switch {
	case errors.As(err, &inv):
		httpx.Fail(c, http.StatusUnprocessableEntity, "invalid_order", inv.Msg)
	case errors.As(err, &st):
		httpx.Fail(c, http.StatusConflict, st.Code, st.Msg)
	default:
		httpx.FailDB(c, err)
	}
}

// scope resolves brand + store for POS routes.
func (h *Handler) scope(c *gin.Context) (brandID, storeID uuid.UUID, ok bool) {
	storeID, ok = tenant.StoreFrom(c, h.q)
	if !ok {
		return
	}
	p := tenant.MustFrom(c)
	if p.BrandID != nil {
		return *p.BrandID, storeID, true
	}
	st, err := h.q.GetStoreByID(c.Request.Context(), storeID)
	if err != nil {
		httpx.FailDB(c, err)
		return uuid.Nil, uuid.Nil, false
	}
	return st.BrandID, storeID, true
}

func parseID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", name+" must be a UUID")
		return uuid.Nil, false
	}
	return id, true
}

// --- store ---

// Store serves GET /pos/store (name, address, bank details for VietQR).
func (h *Handler) Store(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	v, err := h.svc.Store(c.Request.Context(), storeID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

// --- tables (floor plan) ---

// Tables serves GET /pos/tables.
func (h *Handler) Tables(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	plan, err := h.svc.Tables(c.Request.Context(), storeID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, plan)
}

// CreateTable serves POST /pos/tables (store manager and above).
func (h *Handler) CreateTable(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	var in TableInput
	if !httpx.Bind(c, &in) {
		return
	}
	t, err := h.svc.CreateTable(c.Request.Context(), storeID, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

// UpdateTable serves PUT /pos/tables/:id.
func (h *Handler) UpdateTable(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var in TableInput
	if !httpx.Bind(c, &in) {
		return
	}
	t, err := h.svc.UpdateTable(c.Request.Context(), storeID, id, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

// DeleteTable serves DELETE /pos/tables/:id (soft delete).
func (h *Handler) DeleteTable(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteTable(c.Request.Context(), storeID, id); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// --- customers ---

func (h *Handler) LookupCustomer(c *gin.Context) {
	brandID, _, ok := h.scope(c)
	if !ok {
		return
	}
	phone := strings.TrimSpace(c.Query("phone"))
	if phone == "" {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "phone is required")
		return
	}
	v, err := h.svc.LookupCustomer(c.Request.Context(), brandID, phone)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) CreateCustomer(c *gin.Context) {
	brandID, _, ok := h.scope(c)
	if !ok {
		return
	}
	var in CustomerInput
	if !httpx.Bind(c, &in) {
		return
	}
	v, err := h.svc.CreateCustomer(c.Request.Context(), brandID, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

// --- promotions ---

func (h *Handler) Promotion(c *gin.Context) {
	brandID, _, ok := h.scope(c)
	if !ok {
		return
	}
	v, err := h.svc.Promotion(c.Request.Context(), brandID, c.Param("code"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

// --- orders ---

func (h *Handler) Create(c *gin.Context) {
	brandID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	var in CreateInput
	if !httpx.Bind(c, &in) {
		return
	}
	v, err := h.svc.Create(c.Request.Context(), brandID, storeID, tenant.MustFrom(c).UserID, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (h *Handler) Replace(c *gin.Context) {
	brandID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var in CreateInput
	if !httpx.Bind(c, &in) {
		return
	}
	v, err := h.svc.Replace(c.Request.Context(), brandID, storeID, id, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) Pay(c *gin.Context) {
	brandID, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var in PayInput
	if !httpx.Bind(c, &in) {
		return
	}
	v, err := h.svc.Pay(c.Request.Context(), brandID, storeID, id, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) SetStatus(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var in StatusInput
	if !httpx.Bind(c, &in) {
		return
	}
	v, err := h.svc.SetStatus(c.Request.Context(), storeID, id, in.Status)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) Get(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	v, err := h.svc.Get(c.Request.Context(), storeID, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) List(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	f := ListFilter{Date: c.Query("date"), Statuses: splitList(c.Query("status")), Sources: splitList(c.Query("source"))}
	rows, err := h.svc.List(c.Request.Context(), storeID, f)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *Handler) Summary(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	v, err := h.svc.Summary(c.Request.Context(), storeID, c.Query("date"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

type availabilityInput struct {
	Available *bool `json:"available" binding:"required"`
}

// SetAvailability serves PATCH /pos/menu/items/:id/availability (store override).
func (h *Handler) SetAvailability(c *gin.Context) {
	_, storeID, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var in availabilityInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := h.svc.SetStoreAvailability(c.Request.Context(), storeID, id, *in.Available); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"itemId": id, "available": *in.Available})
}

// --- customer app (public) ---

// CreateFromApp serves POST /app/stores/:storeId/orders.
func (h *Handler) CreateFromApp(c *gin.Context) {
	storeID, ok := parseID(c, "storeId")
	if !ok {
		return
	}
	store, err := h.q.GetStoreByID(c.Request.Context(), storeID)
	if err != nil || !store.IsActive {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "store not found")
			return
		}
		httpx.FailDB(c, err)
		return
	}
	var in AppCreateInput
	if !httpx.Bind(c, &in) {
		return
	}
	v, err := h.svc.CreateFromApp(c.Request.Context(), store, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
