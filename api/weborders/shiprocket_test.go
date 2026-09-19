package weborders

import "testing"

func TestParseServiceabilityDataEmptyArray(t *testing.T) {
	t.Parallel()
	list, err := parseServiceabilityBody([]byte(`{"data":[]}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
}

func TestParseServiceabilityDataNull(t *testing.T) {
	t.Parallel()
	list, err := parseServiceabilityBody([]byte(`{"data":null}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
}

func TestParseServiceabilityObject(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"data": {
			"available_courier_companies": [
				{"courier_company_id": 1, "courier_name": "Quick-Flash", "rate": 73, "etd": "30 mins"}
			]
		}
	}`)
	list, err := parseServiceabilityBody(body)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Quick-Flash" || list[0].Rate != 73 {
		t.Fatalf("got %+v", list)
	}
}

func TestParseServiceabilityStringRates(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"data": {
			"available_courier_company": [
				{"courier_company_id": "12", "courier_name": "Quick-Rapido", "rate": "89.5", "freight_charge": "0"}
			]
		}
	}`)
	list, err := parseServiceabilityBody(body)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 1 || list[0].Rate != 89.5 {
		t.Fatalf("got %+v", list)
	}
}

func TestParseServiceabilityDataArrayWithRates(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"status": true,
		"data": [
			{
				"courier_name": "Shiprocket Quick",
				"rates": 193,
				"rto_rates": 193,
				"etd": "Sep 05, 2026",
				"etd_hours": 1,
				"distance": 18.47
			}
		]
	}`)
	list, err := parseServiceabilityBody(body)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Shiprocket Quick" {
		t.Fatalf("got %+v", list)
	}
	if courierFee(list[0]) != 193 {
		t.Fatalf("fee=%v courier=%+v", courierFee(list[0]), list[0])
	}
	best, ok := pickBestCourier(list)
	if !ok || courierFee(best) != 193 {
		t.Fatalf("best=%+v ok=%v", best, ok)
	}
}

func TestPickBestCourierSkipsZeroFee(t *testing.T) {
	t.Parallel()
	best, ok := pickBestCourier([]shiprocketCourier{
		{Name: "Shiprocket Quick", Rate: 0},
		{Name: "Quick-Flash", Rate: 73},
	})
	if !ok || best.Name != "Quick-Flash" {
		t.Fatalf("got %+v ok=%v", best, ok)
	}
}

func TestPickBestCourierPrefersCheapestQuick(t *testing.T) {
	t.Parallel()
	best, ok := pickBestCourier([]shiprocketCourier{
		{Name: "Delhivery Air", Rate: 40},
		{Name: "Quick-Rapido 2WC", Rate: 89},
		{Name: "Quick-Flash", Rate: 73},
	})
	if !ok || best.Name != "Quick-Flash" || best.Rate != 73 {
		t.Fatalf("got %+v ok=%v", best, ok)
	}
}

func TestPickBestCourierFallsBackWhenNoQuick(t *testing.T) {
	t.Parallel()
	best, ok := pickBestCourier([]shiprocketCourier{
		{Name: "Delhivery Surface", Rate: 120},
		{Name: "Xpressbees", Rate: 99},
	})
	if !ok || best.Rate != 99 {
		t.Fatalf("got %+v ok=%v", best, ok)
	}
}

func TestIsQuickCourier(t *testing.T) {
	t.Parallel()
	if !isQuickCourier("Quick-Rapido 2WC") {
		t.Fatal("expected quick")
	}
	if isQuickCourier("Delhivery Air") {
		t.Fatal("delhivery must not be quick")
	}
}
