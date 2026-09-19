package weborders

import "testing"

func TestNormalizeOrderRef(t *testing.T) {
	t.Parallel()
	if got := NormalizeOrderRef("  tgfy-w-ab12  "); got != "TGFY-W-AB12" {
		t.Fatalf("got %q", got)
	}
}

func TestSaveWebOrderRequestValidate(t *testing.T) {
	t.Parallel()
	ok := SaveWebOrderRequest{
		OrderRef:    "TGFY-W-1",
		Fulfillment: "pickup",
		TotalPaise:  50000,
		Lines:       []WebOrderLine{{Name: "Prawn", Qty: 1, PricePaise: 50000}},
	}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := ok
	bad.Fulfillment = "ship"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected fulfillment error")
	}
}

func TestBuildWebOrder(t *testing.T) {
	t.Parallel()
	o := BuildWebOrder("user-1", "pay_1", "order_1", SaveWebOrderRequest{
		OrderRef:    "tgfy-w-zz",
		Fulfillment: "Delivery",
		TotalPaise:  10000,
		Lines:       []WebOrderLine{{Name: "Rice", Qty: 2, PricePaise: 5000}},
	}, 1_700_000_000_000)
	if o.OrderRef != "TGFY-W-ZZ" || o.UserID != "user-1" || o.Fulfillment != "delivery" {
		t.Fatalf("%+v", o)
	}
	if o.PlacedAtUnix != 1_700_000_000_000 || o.Status != "paid" {
		t.Fatalf("%+v", o)
	}
}
