// Package menu manages brand menus (categories, items, option groups) and
// resolves the effective menu of a store (price and availability overrides).
package menu

import (
	"errors"
	"fmt"
)

// Choice is one selectable value inside an option group, e.g. size "L".
type Choice struct {
	Code       string `json:"code" binding:"required,max=32"`
	Name       string `json:"name" binding:"required,max=64"`
	PriceDelta int64  `json:"priceDelta"` // VND added to base price; may be 0
}

// OptionGroup models size / ice / sugar / topping on a menu item.
type OptionGroup struct {
	Code     string   `json:"code" binding:"required,max=32"`
	Name     string   `json:"name" binding:"required,max=64"`
	Type     string   `json:"type" binding:"required,oneof=single multi"`
	Required bool     `json:"required"`
	Choices  []Choice `json:"choices" binding:"required,min=1,dive"`
}

// ValidateOptions enforces the rules the JSON schema tags cannot express.
func ValidateOptions(groups []OptionGroup) error {
	seenGroup := map[string]bool{}
	for _, g := range groups {
		if seenGroup[g.Code] {
			return fmt.Errorf("duplicate option group code %q", g.Code)
		}
		seenGroup[g.Code] = true
		if g.Required && g.Type == "multi" {
			return errors.New("multi-select option groups cannot be required")
		}
		seenChoice := map[string]bool{}
		for _, ch := range g.Choices {
			if seenChoice[ch.Code] {
				return fmt.Errorf("duplicate choice code %q in group %q", ch.Code, g.Code)
			}
			seenChoice[ch.Code] = true
			if ch.PriceDelta < 0 {
				return fmt.Errorf("choice %q in group %q has a negative price delta", ch.Code, g.Code)
			}
		}
	}
	return nil
}
