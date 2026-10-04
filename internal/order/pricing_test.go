package order

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"coffeesos/internal/db"
	"coffeesos/internal/menu"
)

var (
	size = menu.OptionGroup{Code: "size", Name: "Size", Type: "single", Required: true, Choices: []menu.Choice{
		{Code: "m", Name: "M"}, {Code: "l", Name: "L", PriceDelta: 5000},
	}}
	ice = menu.OptionGroup{Code: "ice", Name: "Đá", Type: "single", Required: true, Choices: []menu.Choice{
		{Code: "normal", Name: "Bình thường"}, {Code: "less", Name: "Ít đá"},
	}}
	topping = menu.OptionGroup{Code: "topping", Name: "Topping", Type: "multi", Choices: []menu.Choice{
		{Code: "pearl", Name: "Trân châu", PriceDelta: 8000}, {Code: "jelly", Name: "Thạch", PriceDelta: 5000},
	}}
)

func row(name string, price int64, groups []menu.OptionGroup, available bool) db.ListStoreItemIDsRow {
	opts, _ := json.Marshal(groups)
	return db.ListStoreItemIDsRow{ID: uuid.New(), Name: name, BasePrice: price, Price: price, Options: opts, IsAvailable: available, StoreAvailable: available}
}

func TestPriceLinesMatchesFigmaTicket(t *testing.T) {
	coffee := row("Cà phê sữa đá", 29000, []menu.OptionGroup{size, ice}, true)
	tea := row("Trà đào cam sả", 45000, []menu.OptionGroup{size, ice, topping}, true)
	cake := row("Bánh croissant", 35000, nil, true)
	note := "hâm nóng"

	lines, err := priceLines([]db.ListStoreItemIDsRow{coffee, tea, cake}, []LineInput{
		{ItemID: coffee.ID, Quantity: 1, Choices: []ChoiceInput{{Group: "size", Code: "l"}, {Group: "ice", Code: "less"}}},
		{ItemID: tea.ID, Quantity: 2, Choices: []ChoiceInput{{Group: "size", Code: "m"}, {Group: "ice", Code: "normal"}}},
		{ItemID: cake.ID, Quantity: 1, Note: &note},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		unit, total int64
		text        string
	}{{34000, 34000, "Size L · Ít đá"}, {45000, 90000, "Size M"}, {35000, 35000, ""}}
	for i, w := range want {
		if lines[i].UnitPrice != w.unit || lines[i].LineTotal != w.total || lines[i].OptionsText != w.text {
			t.Errorf("line %d = %+v, want %+v", i, lines[i], w)
		}
	}
	cart, err := totals(lines, &db.Promotion{Code: "SALE10", Type: "percent", Value: 10})
	if err != nil {
		t.Fatal(err)
	}
	if cart.Subtotal != 159000 || cart.Discount != 15900 || cart.Total != 143100 || cart.Points != 14 {
		t.Errorf("totals = %+v", cart)
	}
}

func TestPriceLinesRejectsBadCarts(t *testing.T) {
	coffee := row("Cà phê sữa đá", 29000, []menu.OptionGroup{size, ice}, true)
	soldOut := row("Trà vải", 42000, []menu.OptionGroup{size}, false)
	rows := []db.ListStoreItemIDsRow{coffee, soldOut}

	cases := map[string][]LineInput{
		"missing required": {{ItemID: coffee.ID, Quantity: 1}},
		"unknown choice":   {{ItemID: coffee.ID, Quantity: 1, Choices: []ChoiceInput{{Group: "size", Code: "xl"}, {Group: "ice", Code: "normal"}}}},
		"unknown group":    {{ItemID: coffee.ID, Quantity: 1, Choices: []ChoiceInput{{Group: "size", Code: "m"}, {Group: "ice", Code: "normal"}, {Group: "milk", Code: "oat"}}}},
		"sold out":         {{ItemID: soldOut.ID, Quantity: 1, Choices: []ChoiceInput{{Group: "size", Code: "m"}}}},
		"not on menu":      {{ItemID: uuid.New(), Quantity: 1}},
	}
	for name, in := range cases {
		_, err := priceLines(rows, in)
		var inv *InvalidOrderError
		if !errors.As(err, &inv) {
			t.Errorf("%s: want InvalidOrderError, got %v", name, err)
		}
	}
}

func TestApplyPromotion(t *testing.T) {
	if d, _ := applyPromotion(100000, &db.Promotion{Type: "fixed", Value: 150000}); d != 100000 {
		t.Errorf("fixed discount should cap at subtotal, got %d", d)
	}
	if _, err := applyPromotion(50000, &db.Promotion{Code: "BIG", Type: "percent", Value: 20, MinSubtotal: 100000}); err == nil {
		t.Error("expected min subtotal error")
	}
	if d, _ := applyPromotion(50000, nil); d != 0 {
		t.Errorf("nil promo should give 0, got %d", d)
	}
}

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{"0901 234 567": "0901234567", "+84 901 234 567": "0901234567", "0901-234-567": "0901234567"} {
		if got := normalizePhone(in); got != want {
			t.Errorf("normalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}
