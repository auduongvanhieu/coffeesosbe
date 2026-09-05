package menu

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"coffeesos/internal/db"
	"coffeesos/internal/realtime"
)

const EventItemAvailability = "menu.item.availability"

type Service struct {
	q   *db.Queries
	hub *realtime.Hub
}

func NewService(q *db.Queries, hub *realtime.Hub) *Service {
	return &Service{q: q, hub: hub}
}

// ItemView is the API shape of a menu item; Options is decoded JSON.
type ItemView struct {
	ID          uuid.UUID       `json:"id"`
	CategoryID  uuid.UUID       `json:"categoryId"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	ImageURL    *string         `json:"imageUrl"`
	BasePrice   int64           `json:"basePrice"`
	Options     json.RawMessage `json:"options"`
	IsAvailable bool            `json:"isAvailable"`
	SortOrder   int32           `json:"sortOrder"`
}

// StoreItemView adds the store-resolved price and availability.
type StoreItemView struct {
	ItemView
	Price     int64 `json:"price"`
	Available bool  `json:"available"`
}

type StoreMenu struct {
	StoreID    uuid.UUID         `json:"storeId"`
	Categories []db.MenuCategory `json:"categories"`
	Items      []StoreItemView   `json:"items"`
}

type CreateCategoryInput struct {
	Name      string `json:"name" binding:"required,max=64"`
	SortOrder int32  `json:"sortOrder"`
}

type CreateItemInput struct {
	CategoryID  uuid.UUID     `json:"categoryId" binding:"required"`
	Name        string        `json:"name" binding:"required,max=128"`
	Description *string       `json:"description"`
	ImageURL    *string       `json:"imageUrl" binding:"omitempty,url"`
	BasePrice   int64         `json:"basePrice" binding:"min=0"`
	Options     []OptionGroup `json:"options" binding:"dive"`
	SortOrder   int32         `json:"sortOrder"`
}

func (s *Service) Categories(ctx context.Context, brandID uuid.UUID) ([]db.MenuCategory, error) {
	rows, err := s.q.ListCategoriesByBrand(ctx, brandID)
	if rows == nil {
		rows = []db.MenuCategory{}
	}
	return rows, err
}

func (s *Service) CreateCategory(ctx context.Context, brandID uuid.UUID, in CreateCategoryInput) (db.MenuCategory, error) {
	return s.q.CreateCategory(ctx, db.CreateCategoryParams{BrandID: brandID, Name: in.Name, SortOrder: in.SortOrder})
}

func (s *Service) Items(ctx context.Context, brandID uuid.UUID) ([]ItemView, error) {
	rows, err := s.q.ListItemsByBrand(ctx, brandID)
	if err != nil {
		return nil, err
	}
	out := make([]ItemView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toItemView(r))
	}
	return out, nil
}

func (s *Service) CreateItem(ctx context.Context, brandID uuid.UUID, in CreateItemInput) (ItemView, error) {
	if in.Options == nil {
		in.Options = []OptionGroup{}
	}
	if err := ValidateOptions(in.Options); err != nil {
		return ItemView{}, &ValidationError{Msg: err.Error()}
	}
	opts, err := json.Marshal(in.Options)
	if err != nil {
		return ItemView{}, fmt.Errorf("encode options: %w", err)
	}
	row, err := s.q.CreateItem(ctx, db.CreateItemParams{
		BrandID:     brandID,
		CategoryID:  in.CategoryID,
		Name:        in.Name,
		Description: in.Description,
		ImageUrl:    in.ImageURL,
		BasePrice:   in.BasePrice,
		Options:     opts,
		SortOrder:   in.SortOrder,
	})
	if err != nil {
		return ItemView{}, err
	}
	return toItemView(row), nil
}

// SetAvailability toggles an item brand-wide and notifies every connected
// POS / app of that brand so sold-out items disappear immediately.
func (s *Service) SetAvailability(ctx context.Context, brandID, itemID uuid.UUID, available bool) (ItemView, error) {
	row, err := s.q.SetItemAvailability(ctx, db.SetItemAvailabilityParams{ID: itemID, BrandID: brandID, IsAvailable: available})
	if err != nil {
		return ItemView{}, err
	}
	view := toItemView(row)
	if s.hub != nil {
		s.hub.Broadcast(realtime.BrandRoom(brandID), EventItemAvailability, map[string]any{
			"itemId":      view.ID,
			"isAvailable": view.IsAvailable,
		})
	}
	return view, nil
}

// ForStore returns the effective menu of one store.
func (s *Service) ForStore(ctx context.Context, storeID uuid.UUID) (StoreMenu, error) {
	cats, err := s.q.ListCategoriesByStore(ctx, storeID)
	if err != nil {
		return StoreMenu{}, err
	}
	rows, err := s.q.ListStoreMenu(ctx, storeID)
	if err != nil {
		return StoreMenu{}, err
	}
	if cats == nil {
		cats = []db.MenuCategory{}
	}
	items := make([]StoreItemView, 0, len(rows))
	for _, r := range rows {
		items = append(items, StoreItemView{
			ItemView: ItemView{
				ID: r.ID, CategoryID: r.CategoryID, Name: r.Name, Description: r.Description, ImageURL: r.ImageUrl,
				BasePrice: r.BasePrice, Options: rawJSON(r.Options), IsAvailable: r.IsAvailable, SortOrder: r.SortOrder,
			},
			Price:     r.Price,
			Available: r.StoreAvailable,
		})
	}
	return StoreMenu{StoreID: storeID, Categories: cats, Items: items}, nil
}

// ValidationError marks caller mistakes so the handler can answer 422.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func toItemView(r db.MenuItem) ItemView {
	return ItemView{
		ID: r.ID, CategoryID: r.CategoryID, Name: r.Name, Description: r.Description, ImageURL: r.ImageUrl,
		BasePrice: r.BasePrice, Options: rawJSON(r.Options), IsAvailable: r.IsAvailable, SortOrder: r.SortOrder,
	}
}

func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}
