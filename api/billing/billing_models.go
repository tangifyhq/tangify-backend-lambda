package billing

// All money values are stored in lowest denomination (paise).

const (
	PrefixOrder  = "order"
	PrefixBill   = "bill"
	TableNameBills = "tangify_bills"

	PaymentStatusPending  = "pending"
	PaymentStatusPaid     = "paid"
	PaymentStatusFailed   = "failed"
	PaymentStatusRefunded = "refunded"

	PaymentMethodCash         = "cash"
	PaymentMethodCard         = "card"
	PaymentMethodUPI          = "upi"
	PaymentMethodBankTransfer = "bank_transfer"
	PaymentMethodCheque       = "cheque"
	PaymentMethodOther        = "other"
)

type OrderItemType struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Price    int64  `json:"price"` // In paise
}

type OrderType struct {
	ID          *string          `json:"id,omitempty"`
	Items       *[]OrderItemType `json:"items,omitempty"`
	TotalPrice  *int64           `json:"total_price,omitempty"` // In paise
	Status      *string          `json:"status,omitempty"`
	OrderedAt   *int64           `json:"ordered_at,omitempty"`   // Unix epoch
	ReadyAt     *int64           `json:"ready_at,omitempty"`     // Unix epoch
	CompletedAt *int64           `json:"completed_at,omitempty"` // Unix epoch
	UpdatedAt   *int64           `json:"updated_at,omitempty"`   // Unix epoch
	TableID     *string          `json:"table_id,omitempty"`
	CustomerID  *string          `json:"customer_id,omitempty"`
	StaffID     *string          `json:"staff_id,omitempty"`
}

type BillOrderType struct {
	ID          *string          `json:"id,omitempty"`
	Items       *[]OrderItemType `json:"items,omitempty"`
	TotalPrice  *int64           `json:"total_price,omitempty"` // In paise
	Status      *string          `json:"status,omitempty"`
	OrderedAt   *int64           `json:"ordered_at,omitempty"`   // Unix epoch
	ReadyAt     *int64           `json:"ready_at,omitempty"`     // Unix epoch
	CompletedAt *int64           `json:"completed_at,omitempty"` // Unix epoch
}

type BillType struct {
	ID                   *string          `json:"id,omitempty"`
	Orders               *[]BillOrderType `json:"orders,omitempty"`
	PaymentMethod        *string          `json:"payment_method,omitempty"`
	PaymentStatus        *string          `json:"payment_status,omitempty"`
	CreatedAt            *int64           `json:"created_at,omitempty"`
	UpdatedAt            *int64           `json:"updated_at,omitempty"`
	Discounts            *[]DiscountType  `json:"discounts,omitempty"`
	Taxes                *[]TaxType       `json:"taxes,omitempty"`
	TableID              *string          `json:"table_id,omitempty"`
	CustomerID           *string          `json:"customer_id,omitempty"`
	StaffID              *string          `json:"staff_id,omitempty"`
	TotalTaxInPaise      *int64           `json:"total_tax_in_paise,omitempty"`      // In paise
	TotalDiscountInPaise *int64           `json:"total_discount_in_paise,omitempty"` // In paise
	TotalAmountInPaise   *int64           `json:"total_amount_in_paise,omitempty"`   // In paise
}

type TaxType struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RateInBps     int    `json:"rate_in_bps"`     // In basis points
	AmountInPaise int64  `json:"amount_in_paise"` // In paise
}

type DiscountType struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Amount      int64  `json:"amount"` // In paise
	Description string `json:"description"`
}

type AddOrderRequestType struct {
	BillID     *string         `json:"bill_id,omitempty"` // Optional; if not provided, a new bill will be created
	Items      []OrderItemType `json:"items"`
	OrderedAt  *int64          `json:"ordered_at,omitempty"` // Unix epoch
	TableID    *string         `json:"table_id,omitempty"`
	CustomerID *string         `json:"customer_id,omitempty"`
	StaffID    *string         `json:"staff_id,omitempty"`
}

type ModifyOrderRequestType struct {
	OrderID string          `json:"order_id"`
	Items   []OrderItemType `json:"items"`
}

type UpdateBillRequestType struct {
	BillID        string         `json:"bill_id"`
	Orders        []OrderType    `json:"orders"`
	Discounts     []DiscountType `json:"discounts"`
	Taxes         []TaxType      `json:"taxes"`
	PaymentMethod string         `json:"payment_method"`
	PaymentStatus string         `json:"payment_status"`
}
