package weborders

import (
	"testing"

	"tangify-backend-lambda/menu"
)

func TestToPublicItems(t *testing.T) {
	items := []menu.Item{
		{Status: "ON", Category: "A", Name: "Dalma", Description: "x", IsVeg: true, Price: "180"},
		{Status: "OFF", Category: "B", Name: "Hidden", Price: "10"},
	}
	out := ToPublicItems(items)
	if len(out) != 2 {
		t.Fatalf("expected 2 items (full sheet mirror), got %d", len(out))
	}
	if out[0].Name != "Dalma" || !out[0].IsVeg || out[0].Price != "180" {
		t.Fatalf("unexpected first item: %+v", out[0])
	}
	if out[1].Status != "OFF" {
		t.Fatalf("expected OFF status preserved, got %q", out[1].Status)
	}
}

func TestCountCategories(t *testing.T) {
	items := []PublicMenuItem{
		{Category: "A"},
		{Category: "A"},
		{Category: "B"},
		{Category: ""},
	}
	if n := countCategories(items); n != 3 {
		t.Fatalf("expected 3 categories, got %d", n)
	}
}

func TestCanPublishRole(t *testing.T) {
	if !CanPublishRole("admin") || !CanPublishRole("waiter") || !CanPublishRole("") {
		t.Fatal("admin/waiter/empty should be allowed")
	}
	if CanPublishRole("customer") || CanPublishRole("kitchen") {
		t.Fatal("customer/kitchen should not publish menu")
	}
}
