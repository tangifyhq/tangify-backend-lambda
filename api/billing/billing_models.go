// Package billing — billing_models_v2.go defines flat shapes for DynamoDB: sessions, orders, bills.
// Legacy types live in billing_models.go; migrate callers gradually.
package billing

// --- Where the order was placed (channel) ---

const (
	OrderChannelDiningTable              = "dining_table"
	OrderChannelTakeaway                 = "takeaway"
	OrderChannelWhatsAppQuickDelivery    = "whatsapp_quickdelivery"
	OrderChannelWhatsAppNormalDelivery   = "whatsapp_normaldelivery"
	OrderChannelNeighbourDelivery        = "neighbour_delivery"
)

// --- Table / party session (open → billing → closed) ---

const (
	SessionStatusLive    = "live"    // seats occupied; orders can be added
	SessionStatusBilling = "billing" // waiter opened checkout; Bill row exists
	SessionStatusClosed  = "closed"  // payment settled; table(s) available again
)

// TableSession is one seated party: may span one or many physical tables (joined).
// Opened when the first order is placed; closed after billing is done.
type TableSession struct {
	ID        string   `json:"id"`
	TableIDs  []string `json:"table_ids"`  // single or joined tables for this party
	Status    string   `json:"status"`     // SessionStatus*
	BillID    string   `json:"bill_id,omitempty"` // set when waiter starts close-out (SessionStatusBilling)
	OpenedAt  int64    `json:"opened_at"`  // Unix ms
	ClosedAt  int64    `json:"closed_at,omitempty"`
	UpdatedAt int64    `json:"updated_at,omitempty"`
	VenueID   string   `json:"venue_id,omitempty"`
}

// --- Kitchen: per line item (kitchen view: counts by dish × order) ---

const (
	LineItemStatusPending    = "pending"
	LineItemStatusPreparing  = "preparing"
	LineItemStatusReady      = "ready"
	LineItemStatusServed     = "served"
	LineItemStatusCancelled  = "cancelled"
)

// LineItem is one row on a ticket; each line has its own kitchen status.
type LineItem struct {
	ID       string `json:"id"` // stable id for PATCH item status
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Price    int64  `json:"price"` // paise (per line or per unit — document in API)
	Status   string `json:"status"` // LineItemStatus*
}

// --- Order-level kitchen / plating (FIFO by table uses order + ordered_at) ---

const (
	KitchenStatusPending   = "pending"
	KitchenStatusPreparing = "preparing"
	KitchenStatusReady     = "ready"
	KitchenStatusServed    = "served"
	KitchenStatusCancelled = "cancelled"
)

// Order is one ticket (kitchen + line items). Persisted as its own item; links to TableSession and optional Bill.
type Order struct {
	ID            string     `json:"id"`
	SessionID     string     `json:"session_id"`
	VenueID       string     `json:"venue_id"` // denormalized for GSI_VenueOrdered
	Channel       string     `json:"channel"` // OrderChannel*
	BillID        string     `json:"bill_id,omitempty"` // filled when session is in billing / bill linked
	// SourceTableID attributes this order to one physical table when Session.TableIDs has many (optional).
	SourceTableID string     `json:"source_table_id,omitempty"`
	CustomerID    string     `json:"customer_id,omitempty"`
	StaffID       string     `json:"staff_id,omitempty"`
	Items         []LineItem `json:"items"`
	TotalPrice    int64      `json:"total_price"` // paise
	// KitchenStatus is coarse order state (plating view, FIFO batches).
	KitchenStatus string `json:"kitchen_status"` // KitchenStatus*
	OrderedAt     int64  `json:"ordered_at"`     // Unix ms — FIFO sort key with table/session
	ReadyAt       int64  `json:"ready_at,omitempty"`
	CompletedAt   int64  `json:"completed_at,omitempty"`
	UpdatedAt     int64  `json:"updated_at,omitempty"`
}

// Bill is created when the waiter starts closing the table (checkout); payment totals live here.
type Bill struct {
	ID                   string         `json:"id"`
	SessionID            string         `json:"session_id"`
	TableIDs             []string       `json:"table_ids"` // snapshot for receipt / audit
	CustomerID           string         `json:"customer_id,omitempty"`
	StaffID              string         `json:"staff_id,omitempty"`
	PaymentMethod        string         `json:"payment_method"` // PaymentMethod*
	PaymentStatus        string         `json:"payment_status"` // PaymentStatus*
	CreatedAt            int64          `json:"created_at"`
	UpdatedAt            int64          `json:"updated_at"`
	Discounts            []DiscountType `json:"discounts,omitempty"`
	Taxes                []TaxType      `json:"taxes,omitempty"`
	TotalTaxInPaise      int64          `json:"total_tax_in_paise"`
	TotalDiscountInPaise int64          `json:"total_discount_in_paise"`
	TotalAmountInPaise   int64          `json:"total_amount_in_paise"`
}

// --- Read models (assembled in app; not one Dynamo item) ---

// SessionWithOrders is one live (or billing) party with its orders — waiter “by table” / joined table UI.
type SessionWithOrders struct {
	Session TableSession `json:"session"`
	Orders  []Order      `json:"orders"`
}

// LiveOrdersGroupedResponse is all open sessions with orders, grouped for waiter board.
type LiveOrdersGroupedResponse struct {
	Sessions []SessionWithOrders `json:"sessions"`
}

// KitchenDishCount aggregates one menu line for one order (item-wise count × order id).
type KitchenDishCount struct {
	OrderID   string `json:"order_id"`
	LineItemID string `json:"line_item_id"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	Status    string `json:"status"` // LineItemStatus*
}

// PlatingQueueOrder is a minimal row for FIFO plating by table/session.
type PlatingQueueOrder struct {
	OrderID       string `json:"order_id"`
	SessionID     string `json:"session_id"`
	TableIDs      []string `json:"table_ids"`
	KitchenStatus string `json:"kitchen_status"`
	OrderedAt     int64  `json:"ordered_at"`
}

// --- Waiter flows ---

type CreateSessionAndFirstOrderRequest struct {
	TableIDs   []string   `json:"table_ids"` // one or more joined tables
	Items      []LineItem `json:"items"`     // first ticket lines (ids can be server-generated)
	Channel    string     `json:"channel"`   // OrderChannel*; dining_table for on-prem
	CustomerID *string    `json:"customer_id,omitempty"`
	StaffID    *string    `json:"staff_id,omitempty"`
	OrderedAt  *int64     `json:"ordered_at,omitempty"`
}

type AddOrderToSessionRequest struct {
	SessionID     string     `json:"session_id"`
	Items         []LineItem `json:"items"`
	Channel       string     `json:"channel"`
	SourceTableID *string    `json:"source_table_id,omitempty"` // when joined tables
	StaffID       *string    `json:"staff_id,omitempty"`
	OrderedAt     *int64     `json:"ordered_at,omitempty"`
}

type UpdateOrderRequestV2 struct {
	OrderID       string     `json:"order_id"`
	Items         []LineItem `json:"items,omitempty"`
	TotalPrice    *int64     `json:"total_price,omitempty"`
	KitchenStatus *string    `json:"kitchen_status,omitempty"`
}

type PatchLineItemStatusRequest struct {
	OrderID    string `json:"order_id"`
	LineItemID string `json:"line_item_id"`
	Status     string `json:"status"` // LineItemStatus*
}

type PatchOrderKitchenStatusRequestV2 struct {
	OrderID       string `json:"order_id"`
	KitchenStatus string `json:"kitchen_status"` // KitchenStatus*
}

type ListOrdersForSessionRequest struct {
	SessionID string `json:"session_id"`
}

type StartBillForSessionRequest struct {
	SessionID string `json:"session_id"`
	StaffID   *string `json:"staff_id,omitempty"`
}

type UpdateBillRequestV2 struct {
	BillID        string         `json:"bill_id"`
	Discounts     []DiscountType `json:"discounts,omitempty"`
	Taxes         []TaxType      `json:"taxes,omitempty"`
	PaymentMethod *string        `json:"payment_method,omitempty"`
	PaymentStatus *string        `json:"payment_status,omitempty"`
	// Totals may be server-calculated from orders; optional overrides if you allow
	TotalAmountInPaise *int64 `json:"total_amount_in_paise,omitempty"`
}

type CloseTableRequest struct {
	SessionID string `json:"session_id"`
	BillID    string `json:"bill_id"`
}

// --- Kitchen / plating queries ---

type KitchenItemBoardQuery struct {
	VenueID string `json:"venue_id,omitempty"`
	// Filter to non-terminal line items in app or via GSI on LineItemStatus
}

type PlatingFIFOByTableQuery struct {
	SessionID string `json:"session_id,omitempty"` // if set, FIFO for this party
	TableID   string `json:"table_id,omitempty"`   // else resolve sessions covering this table
	Limit     int32  `json:"limit,omitempty"`
}

type ListOpenKitchenOrdersQueryV2 struct {
	VenueID           string `json:"venue_id,omitempty"`
	TableID           string `json:"table_id,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	Limit             int32  `json:"limit,omitempty"`
	ExclusiveStartKey string `json:"exclusive_start_key,omitempty"`
}
