package billing

import (
	"testing"
)

func TestComputeBillTotals_subtotalAndTax(t *testing.T) {
	items := []LineItemV0{
		{Name: "Dal", Quantity: 2, Price: 15000},
		{Name: "Rice", Quantity: 1, Price: 8000},
	}
	taxes := []TaxType{{ID: "gst", Name: "GST", RateInBps: 500, AmountInPaise: 1900}}

	got, err := computeBillTotals(items, nil, taxes, "", 0, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantSubtotal := int64(38000)
	if got.TotalAmountInPaise != wantSubtotal+1900 {
		t.Fatalf("total=%d want=%d", got.TotalAmountInPaise, wantSubtotal+1900)
	}
	if got.TotalTaxInPaise != 1900 {
		t.Fatalf("tax=%d", got.TotalTaxInPaise)
	}
}

func TestComputeBillTotals_pointsHonorsRequestedCappedByWallet(t *testing.T) {
	items := []LineItemV0{{Name: "Thali", Quantity: 1, Price: 100000}}
	discounts := []DiscountType{{ID: "points", Type: DiscountTypePoints, Amount: 99999}}

	got, err := computeBillTotals(items, discounts, nil, "cust-1", 3, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 3 points * 300 = 900 paise
	if got.TotalDiscountInPaise != 900 {
		t.Fatalf("discount=%d want=900", got.TotalDiscountInPaise)
	}
	if got.PointsRedeemed != 3 {
		t.Fatalf("points=%d want=3", got.PointsRedeemed)
	}
	if got.TotalAmountInPaise != 99100 {
		t.Fatalf("total=%d want=99100", got.TotalAmountInPaise)
	}
}

func TestComputeBillTotals_pointsHonorsExactAmount(t *testing.T) {
	items := []LineItemV0{{Name: "Thali", Quantity: 1, Price: 100000}}
	discounts := []DiscountType{{ID: "points", Type: DiscountTypePoints, Amount: 600}}

	got, err := computeBillTotals(items, discounts, nil, "cust-1", 10, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.PointsRedeemed != 2 {
		t.Fatalf("points=%d want=2", got.PointsRedeemed)
	}
	if got.TotalDiscountInPaise != 600 {
		t.Fatalf("discount=%d want=600", got.TotalDiscountInPaise)
	}
}

func TestComputeBillTotals_pointsCappedBySubtotal(t *testing.T) {
	items := []LineItemV0{{Name: "Snack", Quantity: 1, Price: 500}}
	discounts := []DiscountType{{ID: "points", Type: DiscountTypePoints, Amount: 99999}}

	got, err := computeBillTotals(items, discounts, nil, "cust-1", 10, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 500 paise / 300 = 1 point
	if got.TotalDiscountInPaise != 300 {
		t.Fatalf("discount=%d want=300", got.TotalDiscountInPaise)
	}
	if got.PointsRedeemed != 1 {
		t.Fatalf("points=%d want=1", got.PointsRedeemed)
	}
}

func TestComputeBillTotals_pointsExclusive(t *testing.T) {
	items := []LineItemV0{{Name: "Thali", Quantity: 1, Price: 100000}}
	discounts := []DiscountType{
		{ID: "points", Type: DiscountTypePoints, Amount: 600},
		{ID: "mem", Type: DiscountTypeMembership, Amount: 1000},
	}
	_, err := computeBillTotals(items, discounts, nil, "cust-1", 10, false, nil)
	if err != errPointsExclusive {
		t.Fatalf("err=%v want=%v", err, errPointsExclusive)
	}
}

func TestPointsEarnedFromDiscountedSubtotal(t *testing.T) {
	if got := PointsEarnedFromDiscountedSubtotal(100000, 900); got != 19 {
		t.Fatalf("earned=%d want=19", got)
	}
	if got := PointsEarnedFromDiscountedSubtotal(4900, 0); got != 0 {
		t.Fatalf("earned=%d want=0", got)
	}
}

func TestComputeBillTotals_updateFreezesPoints(t *testing.T) {
	existing := []DiscountType{{
		ID: "points", Type: DiscountTypePoints, Amount: 5000, Description: "2 points redeemed",
	}}
	items := []LineItemV0{{Name: "Snack", Quantity: 1, Price: 10000}}
	discounts := []DiscountType{
		{ID: "points", Type: DiscountTypePoints, Amount: 99999},
		{ID: "comp", Type: "comp", Amount: 1000},
	}

	got, err := computeBillTotals(items, discounts, nil, "", 0, true, existing)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalDiscountInPaise != 6000 {
		t.Fatalf("discount=%d want=6000", got.TotalDiscountInPaise)
	}
}

func TestComputeBillTotals_emptyLineItems(t *testing.T) {
	_, err := computeBillTotals(nil, nil, nil, "", 0, false, nil)
	if err != errEmptyLineItems {
		t.Fatalf("err=%v", err)
	}
}

func TestComputeBillTotals_customerRequiredForPoints(t *testing.T) {
	items := []LineItemV0{{Name: "Meal", Quantity: 1, Price: 50000}}
	discounts := []DiscountType{{ID: "points", Type: DiscountTypePoints, Amount: 600}}

	_, err := computeBillTotals(items, discounts, nil, "", 5, false, nil)
	if err != errCustomerIDRequiredForPoints {
		t.Fatalf("err=%v", err)
	}
}

func TestComputeBillTotals_recomputesTaxAfterDiscount(t *testing.T) {
	// Mirrors POS bill 2026-000703: subtotal ₹827, 70 pts (₹210), tax on taxable.
	items := []LineItemV0{{Name: "Meal", Quantity: 1, Price: 82700}}
	discounts := []DiscountType{{
		ID: "points", Type: DiscountTypePoints, Amount: 21000, Description: "70 points",
	}}
	// Client sent stale tax on full subtotal (the bug).
	staleTaxes := []TaxType{
		{ID: "cgst", Name: "CGST", RateInBps: 250, AmountInPaise: 2068},
		{ID: "sgst", Name: "SGST", RateInBps: 250, AmountInPaise: 2068},
		{ID: "round_off", Name: "Round off", RateInBps: 0, AmountInPaise: 64},
	}

	got, err := computeBillTotals(items, discounts, staleTaxes, "cust-1", 70, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalDiscountInPaise != 21000 {
		t.Fatalf("discount=%d want=21000", got.TotalDiscountInPaise)
	}
	// taxable 61700; CGST/SGST = round(61700*0.025)=1543 each
	var cgst, sgst, roundOff int64
	for _, tax := range got.Taxes {
		switch tax.ID {
		case "cgst":
			cgst = tax.AmountInPaise
		case "sgst":
			sgst = tax.AmountInPaise
		case "round_off":
			roundOff = tax.AmountInPaise
		}
	}
	if cgst != 1543 || sgst != 1543 {
		t.Fatalf("cgst=%d sgst=%d want 1543 each", cgst, sgst)
	}
	// preRound = 61700+1543+1543 = 64786 → ceil ₹648 → roundOff 14
	if roundOff != 14 {
		t.Fatalf("roundOff=%d want=14", roundOff)
	}
	if got.TotalAmountInPaise != 64800 {
		t.Fatalf("total=%d want=64800", got.TotalAmountInPaise)
	}
	if got.TotalTaxInPaise != 1543+1543+14 {
		t.Fatalf("tax=%d", got.TotalTaxInPaise)
	}
}
