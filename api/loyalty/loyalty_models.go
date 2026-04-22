package loyalty

const TableNamePointsWallet = "tangify_points_wallet"

type PointsWallet struct {
	UserID           string `json:"user_id"`
	PointsBalance    int64  `json:"points_balance"`
	LifetimeEarned   int64  `json:"lifetime_earned"`
	LifetimeRedeemed int64  `json:"lifetime_redeemed"`
	UpdatedAt        int64  `json:"updated_at"`
}

type AddPointsRequest struct {
	UserID string `json:"user_id"`
	BillID string `json:"bill_id"`
}

type AddPointsResponse struct {
	UserID         string `json:"user_id"`
	BillID         string `json:"bill_id"`
	PointsEarned   int64  `json:"points_earned"`
	CurrentBalance int64  `json:"current_balance"`
}

type PointsDiscountRequest struct {
	UserID string `json:"user_id"`
}

type PointsDiscountResponse struct {
	UserID               string `json:"user_id"`
	CurrentPoints        int64  `json:"current_points"`
	RedeemablePoints     int64  `json:"redeemable_points"`
	DiscountPer100Points int64  `json:"discount_per_100_points"`
	RedeemableDiscount   int64  `json:"redeemable_discount"`
}

type ApplyDiscountRequest struct {
	UserID string `json:"user_id"`
	BillID string `json:"bill_id"`
}

type ApplyDiscountResponse struct {
	UserID           string `json:"user_id"`
	BillID           string `json:"bill_id"`
	PointsRedeemed   int64  `json:"points_redeemed"`
	DiscountApplied  int64  `json:"discount_applied"`
	RemainingPoints  int64  `json:"remaining_points"`
	UpdatedBillTotal int64  `json:"updated_bill_total"`
}
