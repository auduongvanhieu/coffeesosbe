package order

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"coffeesos/internal/db"
	"coffeesos/internal/menu"
)

// InvalidOrderError is a caller mistake (unknown item, sold out, missing
// required option, bad promo). Handlers answer 422.
type InvalidOrderError struct{ Msg string }

func (e *InvalidOrderError) Error() string { return e.Msg }

func invalid(format string, a ...any) error {
	return &InvalidOrderError{Msg: fmt.Sprintf(format, a...)}
}

// pricedLine is a fully resolved cart line ready to be persisted.
type pricedLine struct {
	ItemID      uuid.UUID
	Name        string
	Quantity    int32
	UnitPrice   int64
	LineTotal   int64
	Choices     []ChoiceView
	OptionsText string
	Note        *string
}

type pricedCart struct {
	Lines    []pricedLine
	Subtotal int64
	Discount int64
	Total    int64
	Points   int32
	Promo    *db.Promotion
}

// priceLines resolves every line against the store menu rows. Prices are
// never trusted from the client.
func priceLines(rows []db.ListStoreItemIDsRow, lines []LineInput) ([]pricedLine, error) {
	byID := make(map[uuid.UUID]db.ListStoreItemIDsRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]pricedLine, 0, len(lines))
	for i, in := range lines {
		row, ok := byID[in.ItemID]
		if !ok {
			return nil, invalid("item %d: not on this store's menu", i+1)
		}
		if !row.StoreAvailable {
			return nil, invalid("%s đã hết món", row.Name)
		}
		var groups []menu.OptionGroup
		if len(row.Options) > 0 {
			if err := json.Unmarshal(row.Options, &groups); err != nil {
				return nil, fmt.Errorf("item %s: decode options: %w", row.ID, err)
			}
		}
		chosen, text, delta, err := resolveChoices(row.Name, groups, in.Choices)
		if err != nil {
			return nil, err
		}
		unit := row.Price + delta
		out = append(out, pricedLine{
			ItemID: row.ID, Name: row.Name, Quantity: in.Quantity, UnitPrice: unit,
			LineTotal: unit * int64(in.Quantity), Choices: chosen, OptionsText: text, Note: in.Note,
		})
	}
	return out, nil
}

// resolveChoices validates the picked choices against the item's option groups
// and builds the human summary ("Size L · Ít đá"). Default (first) choices of
// non-size groups are left out of the summary to keep tickets short.
func resolveChoices(itemName string, groups []menu.OptionGroup, picks []ChoiceInput) ([]ChoiceView, string, int64, error) {
	picked := map[string][]string{}
	for _, p := range picks {
		picked[p.Group] = append(picked[p.Group], p.Code)
	}
	var (
		chosen []ChoiceView
		parts  []string
		delta  int64
	)
	for _, g := range groups {
		codes := picked[g.Code]
		if len(codes) == 0 {
			if g.Required {
				return nil, "", 0, invalid("%s: cần chọn %s", itemName, g.Name)
			}
			continue
		}
		if g.Type == "single" && len(codes) > 1 {
			return nil, "", 0, invalid("%s: %s chỉ chọn một", itemName, g.Name)
		}
		seen := map[string]bool{}
		for _, code := range codes {
			if seen[code] {
				continue
			}
			seen[code] = true
			idx := -1
			for i, ch := range g.Choices {
				if ch.Code == code {
					idx = i
					break
				}
			}
			if idx < 0 {
				return nil, "", 0, invalid("%s: lựa chọn %q không có trong %s", itemName, code, g.Name)
			}
			ch := g.Choices[idx]
			chosen = append(chosen, ChoiceView{Group: g.Code, GroupName: g.Name, Code: ch.Code, Name: ch.Name, PriceDelta: ch.PriceDelta})
			delta += ch.PriceDelta
			if idx == 0 && g.Code != "size" && ch.PriceDelta == 0 {
				continue // default choice: not worth printing
			}
			if utf8.RuneCountInString(ch.Name) <= 3 {
				parts = append(parts, g.Name+" "+ch.Name)
			} else {
				parts = append(parts, ch.Name)
			}
		}
		delete(picked, g.Code)
	}
	for code := range picked {
		return nil, "", 0, invalid("%s: nhóm lựa chọn %q không tồn tại", itemName, code)
	}
	if chosen == nil {
		chosen = []ChoiceView{}
	}
	return chosen, strings.Join(parts, " · "), delta, nil
}

// applyPromotion computes the discount for a subtotal. A nil promo means none.
func applyPromotion(subtotal int64, promo *db.Promotion) (int64, error) {
	if promo == nil {
		return 0, nil
	}
	if subtotal < promo.MinSubtotal {
		return 0, invalid("mã %s cần đơn tối thiểu %dđ", promo.Code, promo.MinSubtotal)
	}
	var d int64
	switch promo.Type {
	case "percent":
		d = subtotal * promo.Value / 100
	case "fixed":
		d = promo.Value
	}
	if d > subtotal {
		d = subtotal
	}
	return d, nil
}

func totals(lines []pricedLine, promo *db.Promotion) (pricedCart, error) {
	var subtotal int64
	for _, l := range lines {
		subtotal += l.LineTotal
	}
	discount, err := applyPromotion(subtotal, promo)
	if err != nil {
		return pricedCart{}, err
	}
	total := subtotal - discount
	return pricedCart{
		Lines: lines, Subtotal: subtotal, Discount: discount, Total: total,
		Points: int32(total / PointsPerVND), Promo: promo,
	}, nil
}
