package menu

import "testing"

func TestValidateOptions(t *testing.T) {
	ok := []OptionGroup{
		{Code: "size", Name: "Size", Type: "single", Required: true, Choices: []Choice{{Code: "m", Name: "M"}, {Code: "l", Name: "L", PriceDelta: 5000}}},
		{Code: "topping", Name: "Topping", Type: "multi", Choices: []Choice{{Code: "pearl", Name: "Trân châu", PriceDelta: 8000}}},
	}
	if err := ValidateOptions(ok); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	cases := map[string][]OptionGroup{
		"duplicate group":  {{Code: "size", Name: "S", Type: "single", Choices: []Choice{{Code: "m", Name: "M"}}}, {Code: "size", Name: "S", Type: "single", Choices: []Choice{{Code: "m", Name: "M"}}}},
		"duplicate choice": {{Code: "size", Name: "S", Type: "single", Choices: []Choice{{Code: "m", Name: "M"}, {Code: "m", Name: "M2"}}}},
		"negative delta":   {{Code: "size", Name: "S", Type: "single", Choices: []Choice{{Code: "m", Name: "M", PriceDelta: -1}}}},
		"required multi":   {{Code: "t", Name: "T", Type: "multi", Required: true, Choices: []Choice{{Code: "a", Name: "A"}}}},
	}
	for name, groups := range cases {
		if err := ValidateOptions(groups); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
