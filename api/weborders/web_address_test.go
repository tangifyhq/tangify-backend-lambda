package weborders

import "testing"

func TestEncodeDecodeWebAddress(t *testing.T) {
	t.Parallel()
	in := &WebAddress{
		UserID:         "u1",
		AddressID:      "a1",
		Label:          "Home",
		HouseFlat:      "12A",
		BuildingStreet: "Main St",
		Pincode:        "562125",
		ReceiverName:   "Ada",
		ReceiverPhone:  "9876543210",
		Lat:            12.86,
		Lng:            77.78,
		PinLabel:       "Near mall",
		UpdatedAt:      1700000000,
	}
	item, err := encodeWebAddress(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := decodeWebAddress(item)
	if err != nil {
		t.Fatal(err)
	}
	if out.Label != in.Label || out.HouseFlat != in.HouseFlat || out.Pincode != in.Pincode {
		t.Fatalf("got %+v", out)
	}
	if out.Lat != in.Lat || out.Lng != in.Lng || out.PinLabel != in.PinLabel {
		t.Fatalf("coords/pin %+v", out)
	}
}
