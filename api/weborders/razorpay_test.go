package weborders

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyPaymentSignature(t *testing.T) {
	t.Parallel()
	secret := "test_secret"
	orderID := "order_ABC"
	paymentID := "pay_XYZ"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(orderID + "|" + paymentID))
	sig := hex.EncodeToString(mac.Sum(nil))

	svc := NewRazorpayService()
	out, err := svc.VerifyPayment(RazorpayConfig{KeyID: "rzp_test", KeySecret: secret}, VerifyRazorpayPaymentRequest{
		RazorpayOrderID:   orderID,
		RazorpayPaymentID: paymentID,
		RazorpaySignature: sig,
	})
	if err != nil || out == nil || !out.Verified {
		t.Fatalf("verify: %v %+v", err, out)
	}
}

func TestVerifyPaymentRejectsBadSignature(t *testing.T) {
	t.Parallel()
	svc := NewRazorpayService()
	_, err := svc.VerifyPayment(RazorpayConfig{KeyID: "rzp_test", KeySecret: "secret"}, VerifyRazorpayPaymentRequest{
		RazorpayOrderID:   "order_1",
		RazorpayPaymentID: "pay_1",
		RazorpaySignature: "deadbeef",
	})
	if err == nil {
		t.Fatal("expected signature error")
	}
}

func TestCreateOrderCallsRazorpay(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order_ref":"TGFY-W-2026-000042","attempt_id":"att-1","year":2026,"sequence":42}`))
	}))
	t.Cleanup(worker.Close)
	t.Setenv(webOrderNumberWorkerURLEnvProd, worker.URL+"/")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orders" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "rzp_test_key" || pass != "secret" {
			t.Fatalf("bad auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"order_123","amount":50000,"currency":"INR","receipt":"TGFY-W-2026-000042","status":"created"}`))
	}))
	t.Cleanup(server.Close)

	svc := NewRazorpayService()
	svc.apiBase = server.URL
	out, err := svc.CreateOrder(context.Background(), RazorpayConfig{
		KeyID:     "rzp_test_key",
		KeySecret: "secret",
	}, CreateRazorpayOrderRequest{
		AmountPaise: 50000,
		AttemptID:   "att-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.OrderID != "order_123" || out.Amount != 50000 || out.KeyID != "rzp_test_key" {
		t.Fatalf("%+v", out)
	}
	if out.OrderRef != "TGFY-W-2026-000042" {
		t.Fatalf("order_ref=%q", out.OrderRef)
	}
}
