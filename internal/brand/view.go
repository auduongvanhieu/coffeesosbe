package brand

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"coffeesos/internal/db"
)

// View is the API shape of a brand. db.Brand.Theme is []byte, which
// encoding/json would base64-encode, so it is exposed as raw JSON here.
type View struct {
	ID        uuid.UUID       `json:"id"`
	Slug      string          `json:"slug"`
	Name      string          `json:"name"`
	Slogan    *string         `json:"slogan"`
	LogoURL   *string         `json:"logoUrl"`
	Theme     json.RawMessage `json:"theme"`
	IsActive  bool            `json:"isActive"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func toView(b db.Brand) View {
	theme := json.RawMessage(b.Theme)
	if len(theme) == 0 {
		theme = json.RawMessage("{}")
	}
	return View{
		ID: b.ID, Slug: b.Slug, Name: b.Name, Slogan: b.Slogan, LogoURL: b.LogoUrl,
		Theme: theme, IsActive: b.IsActive, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}

type Stats struct {
	Stores         int64 `json:"stores"`
	Users          int64 `json:"users"`
	Categories     int64 `json:"categories"`
	Items          int64 `json:"items"`
	AvailableItems int64 `json:"availableItems"`
}
