package weborders

import "testing"

func TestIsWebLoginText(t *testing.T) {
	t.Parallel()
	if !IsWebLoginText("login: web") {
		t.Fatal("expected exact prefill to match")
	}
	if !IsWebLoginText("  LOGIN: WEB  ") {
		t.Fatal("expected case-insensitive match")
	}
	if !IsWebLoginText("Hi\nlogin: web") {
		t.Fatal("expected marker anywhere in the message")
	}
	if IsWebLoginText("I want to redeem points. order: abc") {
		t.Fatal("loyalty prefill must not match web login")
	}
	if IsWebLoginText("login: website") {
		t.Fatal("login: website should not match")
	}
}

func TestContinueLinkMessage(t *testing.T) {
	t.Parallel()
	got := ContinueLinkMessage("https://order.tangify.in/", "ABC12XY")
	want := "Please click on this link - https://order.tangify.in/continue/ABC12XY"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestGenerateNonce(t *testing.T) {
	t.Parallel()
	a, err := generateNonce()
	if err != nil {
		t.Fatal(err)
	}
	b, err := generateNonce()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != loginNonceLength || len(b) != loginNonceLength {
		t.Fatalf("length: %q %q", a, b)
	}
	if a == b {
		t.Fatal("expected distinct nonces")
	}
	for _, c := range a {
		if !containsRune(nonceAlphabet, c) {
			t.Fatalf("invalid char %q in %q", c, a)
		}
	}
}

func TestContinueRequestCode(t *testing.T) {
	t.Parallel()
	r := ContinueRequest{Key: " ab12xy "}
	if r.Code() != "AB12XY" {
		t.Fatalf("got %q", r.Code())
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
