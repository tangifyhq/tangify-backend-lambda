package weborders

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type RazorpayConfig struct {
	KeyID     string
	KeySecret string
}

func RazorpayConfigFromEnv() RazorpayConfig {
	return RazorpayConfig{
		KeyID:     strings.TrimSpace(os.Getenv("RAZORPAY_KEY_ID")),
		KeySecret: strings.TrimSpace(os.Getenv("RAZORPAY_KEY_SECRET")),
	}
}

func (c RazorpayConfig) Validate() error {
	if c.KeyID == "" || c.KeySecret == "" {
		return fmt.Errorf("razorpay is not configured")
	}
	return nil
}

type CreateRazorpayOrderRequest struct {
	AmountPaise int64             `json:"amount_paise"`
	// AttemptID is a stable client checkout key used to allocate TGFY-W-YYYY-NNNNNN.
	AttemptID string            `json:"attempt_id"`
	Receipt   string            `json:"receipt,omitempty"` // deprecated fallback when attempt_id empty
	Notes     map[string]string `json:"notes,omitempty"`
}

type CreateRazorpayOrderResponse struct {
	KeyID    string `json:"key_id"`
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Receipt  string `json:"receipt"`
	OrderRef string `json:"order_ref"`
}

type VerifyRazorpayPaymentRequest struct {
	RazorpayOrderID   string              `json:"razorpay_order_id"`
	RazorpayPaymentID string              `json:"razorpay_payment_id"`
	RazorpaySignature string              `json:"razorpay_signature"`
	Order             SaveWebOrderRequest `json:"order"`
}

type VerifyRazorpayPaymentResponse struct {
	Verified  bool      `json:"verified"`
	OrderID   string    `json:"order_id"`
	PaymentID string    `json:"payment_id"`
	Order     *WebOrder `json:"order,omitempty"`
}

type razorpayOrderAPIResponse struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Receipt  string `json:"receipt"`
	Status   string `json:"status"`
}

type RazorpayService struct {
	httpClient *http.Client
	apiBase    string
}

func NewRazorpayService() *RazorpayService {
	return &RazorpayService{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		apiBase:    "https://api.razorpay.com",
	}
}

func (s *RazorpayService) CreateOrder(
	ctx context.Context,
	cfg RazorpayConfig,
	req CreateRazorpayOrderRequest,
) (*CreateRazorpayOrderResponse, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if req.AmountPaise < 100 {
		return nil, fmt.Errorf("amount must be at least ₹1")
	}

	attemptID := strings.TrimSpace(req.AttemptID)
	if attemptID == "" {
		attemptID = strings.TrimSpace(req.Receipt)
	}
	if attemptID == "" {
		return nil, fmt.Errorf("attempt_id required")
	}

	allocated, err := FetchWebOrderNumber(ctx, attemptID)
	if err != nil {
		return nil, fmt.Errorf("allocate order number: %w", err)
	}
	receipt := allocated.OrderRef
	if len(receipt) > 40 {
		receipt = receipt[:40]
	}

	payload := map[string]any{
		"amount":   req.AmountPaise,
		"currency": "INR",
		"receipt":  receipt,
	}
	notes := map[string]string{}
	for k, v := range req.Notes {
		notes[k] = v
	}
	notes["order_ref"] = allocated.OrderRef
	notes["attempt_id"] = attemptID
	payload["notes"] = notes

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(s.apiBase, "/")+"/v1/orders",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(cfg.KeyID, cfg.KeySecret)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("razorpay order failed: %s", strings.TrimSpace(string(respBody)))
	}

	var created razorpayOrderAPIResponse
	if err := json.Unmarshal(respBody, &created); err != nil {
		return nil, fmt.Errorf("razorpay order decode: %w", err)
	}
	if created.ID == "" {
		return nil, fmt.Errorf("razorpay returned empty order id")
	}

	return &CreateRazorpayOrderResponse{
		KeyID:    cfg.KeyID,
		OrderID:  created.ID,
		Amount:   created.Amount,
		Currency: created.Currency,
		Receipt:  created.Receipt,
		OrderRef: allocated.OrderRef,
	}, nil
}

func (s *RazorpayService) VerifyPayment(cfg RazorpayConfig, req VerifyRazorpayPaymentRequest) (*VerifyRazorpayPaymentResponse, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	orderID := strings.TrimSpace(req.RazorpayOrderID)
	paymentID := strings.TrimSpace(req.RazorpayPaymentID)
	signature := strings.TrimSpace(req.RazorpaySignature)
	if orderID == "" || paymentID == "" || signature == "" {
		return nil, fmt.Errorf("razorpay_order_id, razorpay_payment_id and razorpay_signature are required")
	}

	mac := hmac.New(sha256.New, []byte(cfg.KeySecret))
	_, _ = mac.Write([]byte(orderID + "|" + paymentID))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return nil, fmt.Errorf("invalid payment signature")
	}

	return &VerifyRazorpayPaymentResponse{
		Verified:  true,
		OrderID:   orderID,
		PaymentID: paymentID,
	}, nil
}
