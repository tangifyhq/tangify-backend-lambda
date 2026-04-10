package billing

// DynamoDB table names (see dynamodb/billing-v2/*.json).
const (
	TableNameSessions = "tangify_sessions"
	TableNameOrders   = "tangify_orders"
	TableNameBills    = "tangify_bills"
)

const (
	GSIVenueOpened    = "GSI_VenueOpened"
	GSISessionOrdered = "GSI_SessionOrdered"
	GSIVenueOrdered   = "GSI_VenueOrdered"
	GSISessionBill    = "GSI_SessionBill"
)

// ID prefixes for GenerateUniqueID.
const (
	PrefixSession = "sess"
	PrefixOrder   = "ord"
	PrefixBill    = "bill"
	PrefixLine    = "line"
)

const (
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

type DiscountType struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
}

type TaxType struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RateInBps     int    `json:"rate_in_bps"`
	AmountInPaise int64  `json:"amount_in_paise"`
}
