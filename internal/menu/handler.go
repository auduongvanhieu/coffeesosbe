package menu

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

// --- Admin (brand scope) ---

func (h *Handler) ListCategories(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	rows, err := h.svc.Categories(c.Request.Context(), brandID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *Handler) CreateCategory(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	var in CreateCategoryInput
	if !httpx.Bind(c, &in) {
		return
	}
	row, err := h.svc.CreateCategory(c.Request.Context(), brandID, in)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (h *Handler) ListItems(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	rows, err := h.svc.Items(c.Request.Context(), brandID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *Handler) CreateItem(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	var in CreateItemInput
	if !httpx.Bind(c, &in) {
		return
	}
	row, err := h.svc.CreateItem(c.Request.Context(), brandID, in)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusUnprocessableEntity, "invalid_options", ve.Msg)
			return
		}
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusCreated, row)
}

type availabilityInput struct {
	IsAvailable *bool `json:"isAvailable" binding:"required"`
}

func (h *Handler) SetAvailability(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	itemID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "item id must be a UUID")
		return
	}
	var in availabilityInput
	if !httpx.Bind(c, &in) {
		return
	}
	row, err := h.svc.SetAvailability(c.Request.Context(), brandID, itemID, *in.IsAvailable)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

// --- POS (store scope) ---

// StoreMenu serves GET /api/v1/pos/menu for the caller's store. Managers and
// owners (who are not pinned to a store) pass X-Store-ID.
func (h *Handler) StoreMenu(c *gin.Context) {
	storeID, ok := tenant.StoreFrom(c, h.q)
	if !ok {
		return
	}
	menu, err := h.svc.ForStore(c.Request.Context(), storeID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, menu)
}

// --- Customer app (public) ---

// PublicStoreMenu serves GET /api/v1/app/stores/:storeId/menu.
func (h *Handler) PublicStoreMenu(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "store id must be a UUID")
		return
	}
	store, err := h.q.GetStoreByID(c.Request.Context(), storeID)
	if err != nil || !store.IsActive {
		httpx.Fail(c, http.StatusNotFound, "not_found", "store not found")
		return
	}
	menu, err := h.svc.ForStore(c.Request.Context(), storeID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, menu)
}

func (h *Handler) UpdateCategory(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "category id must be a UUID")
		return
	}
	var in UpdateCategoryInput
	if !httpx.Bind(c, &in) {
		return
	}
	row, err := h.svc.UpdateCategory(c.Request.Context(), brandID, id, in)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *Handler) UpdateItem(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "item id must be a UUID")
		return
	}
	var in CreateItemInput
	if !httpx.Bind(c, &in) {
		return
	}
	row, err := h.svc.UpdateItem(c.Request.Context(), brandID, id, in)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			httpx.Fail(c, http.StatusUnprocessableEntity, "invalid_options", ve.Msg)
			return
		}
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}
