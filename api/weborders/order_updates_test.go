package weborders

import "testing"

func TestParseOrderUpdatesRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"order_updates: web-TGFY-W-UGBD90", "TGFY-W-UGBD90"},
		{"ORDER_UPDATES: WEB-tgfy-w-abc12", "TGFY-W-ABC12"},
		{"order_updates: TGFY-W-UGBD90", "TGFY-W-UGBD90"},
		{"Hi\norder_updates: web-TGFY-W-ZZ", "TGFY-W-ZZ"},
		{"login: web", ""},
		{"order: freeflow:abc", ""},
	}
	for _, c := range cases {
		if got := ParseOrderUpdatesRef(c.in); got != c.want {
			t.Fatalf("ParseOrderUpdatesRef(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestOrderDetailsLinkMessage(t *testing.T) {
	t.Parallel()
	got := OrderDetailsLinkMessage("https://order.tangify.in/", "TGFY-W-UGBD90")
	want := "Please click on this link - https://order.tangify.in/orders/TGFY-W-UGBD90"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
