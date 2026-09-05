// Package seed creates the first platform admin and, optionally, the demo
// tenant "Daily Bean" so every client has data to render on day one.
// It is idempotent: existing records are left untouched.
package seed

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"coffeesos/internal/auth"
	"coffeesos/internal/config"
	"coffeesos/internal/db"
	"coffeesos/internal/menu"
	"coffeesos/internal/tenant"
)

const (
	DemoBrandSlug  = "daily-bean"
	DemoOwnerEmail = "owner@dailybean.local"
	DemoStaffEmail = "staff@dailybean.local"
	DemoPassword   = "DailyBean123"
)

func Run(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger) error {
	q := db.New(pool)

	if err := seedPlatformAdmin(ctx, q, cfg, log); err != nil {
		return err
	}
	if cfg.SeedDemo {
		if err := seedDemoBrand(ctx, q, log); err != nil {
			return err
		}
	}
	return nil
}

func seedPlatformAdmin(ctx context.Context, q *db.Queries, cfg *config.Config, log *slog.Logger) error {
	n, err := q.CountUsersByRole(ctx, tenant.RolePlatformAdmin)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if n > 0 {
		log.Info("seed: platform admin already exists, skipping")
		return nil
	}
	password := cfg.SeedAdminPassword
	generated := false
	if password == "" {
		password = randomPassword()
		generated = true
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	email := strings.ToLower(cfg.SeedAdminEmail)
	if _, err := q.CreateUser(ctx, db.CreateUserParams{
		RoleCode: tenant.RolePlatformAdmin, Email: &email, FullName: "Platform Admin", PasswordHash: &hash,
	}); err != nil {
		return fmt.Errorf("create platform admin: %w", err)
	}
	if generated {
		// Printed exactly once; store it in a password manager.
		fmt.Printf("\n==> Platform admin created\n    email:    %s\n    password: %s\n    (generated because SEED_ADMIN_PASSWORD was empty; change it soon)\n\n", email, password)
	} else {
		log.Info("seed: platform admin created", "email", email)
	}
	return nil
}

func seedDemoBrand(ctx context.Context, q *db.Queries, log *slog.Logger) error {
	if _, err := q.GetBrandBySlug(ctx, DemoBrandSlug); err == nil {
		log.Info("seed: demo brand already exists, skipping")
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lookup demo brand: %w", err)
	}

	theme, _ := json.Marshal(map[string]string{
		"primary":      "#C05B33", // terracotta
		"primaryLight": "#F1D0BD",
		"success":      "#1C7D87",
		"background":   "#F9F5F1",
		"text":         "#2F2D2C",
		"muted":        "#9B9B9B",
		"font":         "Be Vietnam Pro",
	})
	slogan := "Ly cà phê mỗi ngày của bạn"
	b, err := q.CreateBrand(ctx, db.CreateBrandParams{Slug: DemoBrandSlug, Name: "Daily Bean", Slogan: &slogan, Theme: theme})
	if err != nil {
		return fmt.Errorf("create demo brand: %w", err)
	}

	addr := "123 Võ Văn Tần, Quận 3, TP.HCM"
	phone := "0901234567"
	store, err := q.CreateStore(ctx, db.CreateStoreParams{BrandID: b.ID, Code: "q3", Name: "Daily Bean · Quận 3", Address: &addr, Phone: &phone})
	if err != nil {
		return fmt.Errorf("create demo store: %w", err)
	}

	hash, err := auth.HashPassword(DemoPassword)
	if err != nil {
		return err
	}
	owner := DemoOwnerEmail
	if _, err := q.CreateUser(ctx, db.CreateUserParams{
		BrandID: &b.ID, RoleCode: tenant.RoleBrandOwner, Email: &owner, FullName: "Chủ quán Daily Bean", PasswordHash: &hash,
	}); err != nil {
		return fmt.Errorf("create demo owner: %w", err)
	}
	staff := DemoStaffEmail
	if _, err := q.CreateUser(ctx, db.CreateUserParams{
		BrandID: &b.ID, StoreID: &store.ID, RoleCode: tenant.RoleStaff, Email: &staff, FullName: "Nhân viên Quận 3", PasswordHash: &hash,
	}); err != nil {
		return fmt.Errorf("create demo staff: %w", err)
	}

	cats := map[string]uuid.UUID{}
	for i, name := range []string{"Cà phê", "Trà", "Đá xay", "Bánh ngọt"} {
		c, err := q.CreateCategory(ctx, db.CreateCategoryParams{BrandID: b.ID, Name: name, SortOrder: int32(i)})
		if err != nil {
			return fmt.Errorf("create category %q: %w", name, err)
		}
		cats[name] = c.ID
	}

	size := menu.OptionGroup{Code: "size", Name: "Size", Type: "single", Required: true, Choices: []menu.Choice{
		{Code: "m", Name: "M"}, {Code: "l", Name: "L", PriceDelta: 5000},
	}}
	ice := menu.OptionGroup{Code: "ice", Name: "Đá", Type: "single", Required: true, Choices: []menu.Choice{
		{Code: "normal", Name: "Bình thường"}, {Code: "less", Name: "Ít đá"}, {Code: "none", Name: "Không đá"},
	}}
	sugar := menu.OptionGroup{Code: "sugar", Name: "Đường", Type: "single", Required: true, Choices: []menu.Choice{
		{Code: "100", Name: "100%"}, {Code: "70", Name: "70%"}, {Code: "50", Name: "50%"}, {Code: "0", Name: "Không đường"},
	}}
	topping := menu.OptionGroup{Code: "topping", Name: "Topping", Type: "multi", Choices: []menu.Choice{
		{Code: "pearl", Name: "Trân châu", PriceDelta: 8000}, {Code: "jelly", Name: "Thạch", PriceDelta: 5000},
	}}
	drink := []menu.OptionGroup{size, ice, sugar}
	tea := []menu.OptionGroup{size, ice, sugar, topping}

	items := []struct {
		cat       string
		name      string
		price     int64
		opts      []menu.OptionGroup
		available bool
	}{
		{"Cà phê", "Cà phê sữa đá", 29000, drink, true},
		{"Cà phê", "Bạc xỉu", 32000, drink, true},
		{"Cà phê", "Cà phê đen", 25000, drink, true},
		{"Trà", "Trà đào cam sả", 45000, tea, true},
		{"Trà", "Trà vải", 42000, tea, false}, // sold out in the Figma mockups
		{"Đá xay", "Cookies & Cream", 55000, []menu.OptionGroup{size, topping}, true},
		{"Bánh ngọt", "Bánh croissant", 35000, nil, true},
	}
	for i, it := range items {
		opts, _ := json.Marshal(nonNil(it.opts))
		row, err := q.CreateItem(ctx, db.CreateItemParams{
			BrandID: b.ID, CategoryID: cats[it.cat], Name: it.name, BasePrice: it.price, Options: opts, SortOrder: int32(i),
		})
		if err != nil {
			return fmt.Errorf("create item %q: %w", it.name, err)
		}
		if !it.available {
			if _, err := q.SetItemAvailability(ctx, db.SetItemAvailabilityParams{ID: row.ID, BrandID: b.ID, IsAvailable: false}); err != nil {
				return err
			}
		}
	}

	log.Info("seed: demo brand created",
		"brand", b.Name, "brandId", b.ID, "storeId", store.ID,
		"owner", DemoOwnerEmail, "staff", DemoStaffEmail, "password", DemoPassword)
	return nil
}

func nonNil(g []menu.OptionGroup) []menu.OptionGroup {
	if g == nil {
		return []menu.OptionGroup{}
	}
	return g
}

func randomPassword() string {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
