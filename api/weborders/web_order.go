package weborders

import (
	"fmt"
	"strings"
	"time"
)

const TableNameWebOrders = "tangify-web-orders"
const GSIUserPlaced = "GSI_UserPlaced"

type WebOrderLine struct {
	Name       string `json:"name"`
	Qty        int    `json:"qty"`
	PricePaise int64  `json:"price_paise"`
}

// WebOrder is a paid web checkout order (DynamoDB + API).
// All money fields are integer paise (never rupees).
type WebOrder struct {
	OrderRef         string         `json:"order_ref"`
	ID               string         `json:"id"`
	UserID           string         `json:"user_id"`
	PaymentID        string         `json:"payment_id"`
	RazorpayOrderID  string         `json:"razorpay_order_id"`
	PlacedAt         string         `json:"placed_at"`
	PlacedAtUnix     int64          `json:"-"`
	Fulfillment      string         `json:"fulfillment"`
	ItemsTotalPaise  int64          `json:"items_total_paise"`
	CgstPaise        int64          `json:"cgst_paise"`
	SgstPaise        int64          `json:"sgst_paise"`
	DeliveryFeePaise int64          `json:"delivery_fee_paise"`
	RoundOffPaise    int64          `json:"round_off_paise"`
	TotalPaise       int64          `json:"total_paise"`
	Lines            []WebOrderLine `json:"lines"`
	AddressLabel     string         `json:"address_label,omitempty"`
	AddressSummary   string         `json:"address_summary,omitempty"`
	ReceiverName     string         `json:"receiver_name,omitempty"`
	ReceiverPhone    string         `json:"receiver_phone,omitempty"`
	Status           string         `json:"status"`
}

type SaveWebOrderRequest struct {
	OrderRef         string         `json:"order_ref"`
	Fulfillment      string         `json:"fulfillment"`
	ItemsTotalPaise  int64          `json:"items_total_paise"`
	CgstPaise        int64          `json:"cgst_paise"`
	SgstPaise        int64          `json:"sgst_paise"`
	DeliveryFeePaise int64          `json:"delivery_fee_paise"`
	RoundOffPaise    int64          `json:"round_off_paise"`
	TotalPaise       int64          `json:"total_paise"`
	Lines            []WebOrderLine `json:"lines"`
	AddressLabel     string         `json:"address_label,omitempty"`
	AddressSummary   string         `json:"address_summary,omitempty"`
	ReceiverName     string         `json:"receiver_name,omitempty"`
	ReceiverPhone    string         `json:"receiver_phone,omitempty"`
}

func NormalizeOrderRef(ref string) string {
	return strings.ToUpper(strings.TrimSpace(ref))
}

func (r SaveWebOrderRequest) Validate() error {
	if NormalizeOrderRef(r.OrderRef) == "" {
		return fmt.Errorf("order_ref required")
	}
	ful := strings.ToLower(strings.TrimSpace(r.Fulfillment))
	if ful != "delivery" && ful != "pickup" {
		return fmt.Errorf("fulfillment must be delivery or pickup")
	}
	if r.TotalPaise < 100 {
		return fmt.Errorf("total_paise must be at least 100")
	}
	if r.DeliveryFeePaise < 0 {
		return fmt.Errorf("delivery_fee_paise must be >= 0")
	}
	if r.ItemsTotalPaise < 0 {
		return fmt.Errorf("items_total_paise must be >= 0")
	}
	if r.CgstPaise < 0 || r.SgstPaise < 0 || r.RoundOffPaise < 0 {
		return fmt.Errorf("tax and round_off must be >= 0")
	}
	if ful == "pickup" && r.DeliveryFeePaise != 0 {
		return fmt.Errorf("delivery_fee_paise must be 0 for pickup")
	}
	if len(r.Lines) == 0 {
		return fmt.Errorf("lines required")
	}
	for i, line := range r.Lines {
		if strings.TrimSpace(line.Name) == "" || line.Qty < 1 || line.PricePaise < 0 {
			return fmt.Errorf("invalid line at index %d", i)
		}
	}
	return nil
}

func BuildWebOrder(userID string, paymentID, razorpayOrderID string, req SaveWebOrderRequest, nowUnixMs int64) *WebOrder {
	if nowUnixMs <= 0 {
		nowUnixMs = time.Now().UnixMilli()
	}
	ful := strings.ToLower(strings.TrimSpace(req.Fulfillment))
	lines := make([]WebOrderLine, 0, len(req.Lines))
	var linesSum int64
	for _, l := range req.Lines {
		lines = append(lines, WebOrderLine{
			Name:       strings.TrimSpace(l.Name),
			Qty:        l.Qty,
			PricePaise: l.PricePaise,
		})
		linesSum += l.PricePaise * int64(l.Qty)
	}
	itemsTotal := req.ItemsTotalPaise
	if itemsTotal <= 0 {
		itemsTotal = linesSum
	}
	deliveryFee := req.DeliveryFeePaise
	if ful == "pickup" {
		deliveryFee = 0
	}
	cgst := req.CgstPaise
	sgst := req.SgstPaise
	roundOff := req.RoundOffPaise
	total := req.TotalPaise
	if total <= 0 {
		total = itemsTotal + cgst + sgst + deliveryFee + roundOff
	}
	ref := NormalizeOrderRef(req.OrderRef)
	return &WebOrder{
		OrderRef:         ref,
		ID:               ref,
		UserID:           strings.TrimSpace(userID),
		PaymentID:        strings.TrimSpace(paymentID),
		RazorpayOrderID:  strings.TrimSpace(razorpayOrderID),
		PlacedAt:         time.UnixMilli(nowUnixMs).UTC().Format(time.RFC3339),
		PlacedAtUnix:     nowUnixMs,
		Fulfillment:      ful,
		ItemsTotalPaise:  itemsTotal,
		CgstPaise:        cgst,
		SgstPaise:        sgst,
		DeliveryFeePaise: deliveryFee,
		RoundOffPaise:    roundOff,
		TotalPaise:       total,
		Lines:            lines,
		AddressLabel:     strings.TrimSpace(req.AddressLabel),
		AddressSummary:   strings.TrimSpace(req.AddressSummary),
		ReceiverName:     strings.TrimSpace(req.ReceiverName),
		ReceiverPhone:    strings.TrimSpace(req.ReceiverPhone),
		Status:           "paid",
	}
}
