package menu

import (
	"encoding/json"
	"testing"
)

func TestParseIsVeg(t *testing.T) {
	t.Parallel()
	if !parseIsVeg("veg") || !parseIsVeg("TRUE") || !parseIsVeg("yes") {
		t.Fatal("expected veg/TRUE/yes to be veg")
	}
	if parseIsVeg("FALSE") || parseIsVeg("nonveg") || parseIsVeg("") {
		t.Fatal("expected FALSE/nonveg/empty to be non-veg")
	}
}

func TestMapOrderingRowByHeader(t *testing.T) {
	t.Parallel()
	header := headerKeys([]json.RawMessage{
		json.RawMessage(`"status"`),
		json.RawMessage(`"category"`),
		json.RawMessage(`"name"`),
		json.RawMessage(`"description"`),
		json.RawMessage(`"is_veg"`),
		json.RawMessage(`"price"`),
		json.RawMessage(`"internal_name"`),
	})
	row := []json.RawMessage{
		json.RawMessage(`"on"`),
		json.RawMessage(`"BAAR BAAR"`),
		json.RawMessage(`"HERITAGE GOAT MUTTON KASSA"`),
		json.RawMessage(`"Slow-cooked mutton"`),
		json.RawMessage(`"FALSE"`),
		json.RawMessage(`"609"`),
		json.RawMessage(`"heritage_goat_mutton_kassa"`),
	}
	item := mapOrderingRow(header, row)
	if item.Name != "HERITAGE GOAT MUTTON KASSA" {
		t.Fatalf("name %q", item.Name)
	}
	if item.IsVeg {
		t.Fatal("FALSE should not be veg")
	}
	if item.Price != "609" || item.Category != "BAAR BAAR" {
		t.Fatalf("got %+v", item)
	}
	if item.Status != "on" {
		t.Fatalf("status %q", item.Status)
	}
}

func TestMapOrderingRowVegTrue(t *testing.T) {
	t.Parallel()
	header := []string{"status", "category", "name", "description", "is_veg", "price"}
	row := []json.RawMessage{
		json.RawMessage(`"ON"`),
		json.RawMessage(`"Sides"`),
		json.RawMessage(`"Dalma"`),
		json.RawMessage(`""`),
		json.RawMessage(`"TRUE"`),
		json.RawMessage(`"180"`),
	}
	item := mapOrderingRow(header, row)
	if !item.IsVeg {
		t.Fatal("TRUE should be veg")
	}
}
