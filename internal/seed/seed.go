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
	"coffeesos/internal/order"
	"coffeesos/internal/tenant"
)

const (
	DemoBrandSlug  = "daily-bean"
	DemoOwnerEmail = "owner@dailybean.local"
	DemoStaffEmail = "staff@dailybean.local"
	DemoPassword   = "DailyBean123"
	DemoStaffPIN   = "1234"
	DemoPromoCode  = "SALE10"
	DemoCustomer   = "0901234567"
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
		if err := seedDemoExtras(ctx, pool, q, log); err != nil {
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

// seedDemoExtras adds the POS-phase demo data (staff PIN, bank account for
// VietQR, a loyalty customer, a promo code, a few app orders). It runs on
// every seed and only fills what is missing, so existing deployments pick it up.
func seedDemoExtras(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, log *slog.Logger) error {
	b, err := q.GetBrandBySlug(ctx, DemoBrandSlug)
	if err != nil {
		return fmt.Errorf("lookup demo brand: %w", err)
	}
	stores, err := q.ListStoresByBrand(ctx, b.ID)
	if err != nil || len(stores) == 0 {
		return fmt.Errorf("demo store missing: %w", err)
	}
	store := stores[0]

	// Staff PIN
	if u, err := q.GetUserByEmail(ctx, DemoStaffEmail); err == nil && u.PinHash == nil {
		hash, err := auth.HashPassword(DemoStaffPIN)
		if err != nil {
			return err
		}
		if err := q.SetUserPin(ctx, db.SetUserPinParams{ID: u.ID, BrandID: &b.ID, PinHash: &hash}); err != nil {
			return fmt.Errorf("set staff pin: %w", err)
		}
		log.Info("seed: staff PIN set", "email", DemoStaffEmail, "pin", DemoStaffPIN)
	}

	// Bank account shown as VietQR on the payment screen
	if store.BankAccount == nil {
		bin, code, acc, holder := "970436", "VCB", "0011002234", "CAFE NHA MINH"
		if store, err = q.UpdateStoreBank(ctx, db.UpdateStoreBankParams{ID: store.ID, BankBin: &bin, BankCode: &code, BankAccount: &acc, BankHolder: &holder}); err != nil {
			return fmt.Errorf("set store bank: %w", err)
		}
	}

	// Loyalty customer (gold tier) and promo code
	if _, err := q.GetCustomerByPhone(ctx, db.GetCustomerByPhoneParams{BrandID: b.ID, Phone: DemoCustomer}); errors.Is(err, pgx.ErrNoRows) {
		if _, err := q.CreateCustomer(ctx, db.CreateCustomerParams{BrandID: b.ID, Phone: DemoCustomer, Name: "Nguyễn An", Points: 120}); err != nil {
			return fmt.Errorf("create demo customer: %w", err)
		}
	}
	if _, err := q.GetPromotionByCode(ctx, db.GetPromotionByCodeParams{BrandID: b.ID, Upper: DemoPromoCode}); errors.Is(err, pgx.ErrNoRows) {
		if _, err := q.CreatePromotion(ctx, db.CreatePromotionParams{BrandID: b.ID, Code: DemoPromoCode, Name: "Giảm 10%", Type: "percent", Value: 10}); err != nil {
			return fmt.Errorf("create demo promotion: %w", err)
		}
	}

	// Floor plan: 10 tables so the overview screen has something to show.
	if n, err := q.CountStoreTables(ctx, store.ID); err == nil && n == 0 {
		zones := []struct {
			name  string
			count int
			seats int16
		}{{"Trong nhà", 6, 4}, {"Ngoài sân", 4, 2}}
		i := 0
		for _, z := range zones {
			for k := 0; k < z.count; k++ {
				i++
				if _, err := q.CreateStoreTable(ctx, db.CreateStoreTableParams{
					StoreID: store.ID, Name: fmt.Sprintf("Bàn %02d", i), Zone: z.name, Seats: z.seats, SortOrder: int32(i),
				}); err != nil {
					return fmt.Errorf("create table %d: %w", i, err)
				}
			}
		}
		log.Info("seed: tables created", "count", i)
	}

	// A few app orders so the "Đơn từ app" screen is not empty
	n, err := q.CountOrdersByStore(ctx, store.ID)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	items, err := q.ListStoreMenu(ctx, store.ID)
	if err != nil {
		return err
	}
	byName := map[string]uuid.UUID{}
	for _, it := range items {
		byName[it.Name] = it.ID
	}
	svc := order.NewService(pool, q, nil)
	momo := "momo"
	samples := []struct {
		name, phone string
		paid        bool
		method      *string
		status      string
		items       []order.LineInput
	}{
		{"Nguyễn An", DemoCustomer, false, nil, order.StatusPending, []order.LineInput{
			{ItemID: byName["Cà phê sữa đá"], Quantity: 1, Choices: []order.ChoiceInput{{Group: "size", Code: "l"}, {Group: "ice", Code: "less"}, {Group: "sugar", Code: "100"}}},
			{ItemID: byName["Trà đào cam sả"], Quantity: 2, Choices: []order.ChoiceInput{{Group: "size", Code: "m"}, {Group: "ice", Code: "normal"}, {Group: "sugar", Code: "100"}}},
		}},
		{"Trần Bình", "0912345678", true, &momo, order.StatusPending, []order.LineInput{
			{ItemID: byName["Cookies & Cream"], Quantity: 1, Choices: []order.ChoiceInput{{Group: "size", Code: "m"}}},
			{ItemID: byName["Bánh croissant"], Quantity: 1},
		}},
		{"Lê Chi", "0987654321", true, &momo, order.StatusReady, []order.LineInput{
			{ItemID: byName["Bạc xỉu"], Quantity: 2, Choices: []order.ChoiceInput{{Group: "size", Code: "m"}, {Group: "ice", Code: "normal"}, {Group: "sugar", Code: "70"}}},
		}},
	}
	for _, sm := range samples {
		v, err := svc.CreateFromApp(ctx, store, order.AppCreateInput{
			CustomerName: sm.name, CustomerPhone: sm.phone, OrderType: order.TypePickup,
			PaymentMethod: sm.method, Paid: sm.paid, Items: sm.items,
		})
		if err != nil {
			return fmt.Errorf("create sample app order: %w", err)
		}
		if sm.status != order.StatusPending {
			if _, err := svc.SetStatus(ctx, store.ID, v.ID, order.StatusPreparing); err != nil {
				return err
			}
			if sm.status == order.StatusReady {
				if _, err := svc.SetStatus(ctx, store.ID, v.ID, order.StatusReady); err != nil {
					return err
				}
			}
		}
	}
	log.Info("seed: demo app orders created", "count", len(samples))
	return nil
}
