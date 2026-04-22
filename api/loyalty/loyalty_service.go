package loyalty

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"tangify-backend-lambda/billing"
)

const (
	earnPointsPerSpendPaise = int64(25000) // Rs 250
	earnPointsAmount        = int64(10)
	redeemBlockPoints       = int64(100)
	defaultDiscountPaise    = int64(25000) // Rs 250
	loyaltyDiscountID       = "loyalty"
	loyaltyDiscountType     = "loyalty"
)

type Service struct {
	repo     *Repository
	billRepo *billing.Repository
}

func NewService(repo *Repository, billRepo *billing.Repository) *Service {
	return &Service{repo: repo, billRepo: billRepo}
}

func discountPer100Points() int64 {
	raw := strings.TrimSpace(os.Getenv("LOYALTY_DISCOUNT_PER_100_POINTS_PAISE"))
	if raw == "" {
		return defaultDiscountPaise
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return defaultDiscountPaise
	}
	return v
}

func pointsFromSpend(totalPaise int64) int64 {
	if totalPaise <= 0 {
		return 0
	}
	return (totalPaise / earnPointsPerSpendPaise) * earnPointsAmount
}

func (s *Service) AddPointsForBill(ctx context.Context, req AddPointsRequest, now int64) (*AddPointsResponse, error) {
	if strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.BillID) == "" {
		return nil, fmt.Errorf("user_id and bill_id required")
	}
	b, err := s.billRepo.GetBill(ctx, req.BillID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("bill not found")
	}

	w, err := s.repo.GetWallet(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if b.LoyaltyUserID != "" && b.LoyaltyUserID != req.UserID {
		return nil, fmt.Errorf("bill already associated with another user")
	}
	// Idempotent: if already awarded for this bill, return current balance.
	if b.LoyaltyPointsEarned > 0 || b.LoyaltyPointsProcessed {
		return &AddPointsResponse{
			UserID:         req.UserID,
			BillID:         req.BillID,
			PointsEarned:   b.LoyaltyPointsEarned,
			CurrentBalance: w.PointsBalance,
		}, nil
	}

	earned := pointsFromSpend(b.TotalAmountInPaise)
	w.PointsBalance += earned
	w.LifetimeEarned += earned
	w.UpdatedAt = now
	if err := s.repo.PutWallet(ctx, w); err != nil {
		return nil, err
	}

	b.LoyaltyUserID = req.UserID
	b.LoyaltyPointsEarned = earned
	b.LoyaltyPointsProcessed = true
	b.UpdatedAt = now
	if err := s.billRepo.PutBill(ctx, b); err != nil {
		return nil, err
	}

	return &AddPointsResponse{
		UserID:         req.UserID,
		BillID:         req.BillID,
		PointsEarned:   earned,
		CurrentBalance: w.PointsBalance,
	}, nil
}

func (s *Service) GetPointsDiscount(ctx context.Context, req PointsDiscountRequest) (*PointsDiscountResponse, error) {
	if strings.TrimSpace(req.UserID) == "" {
		return nil, fmt.Errorf("user_id required")
	}
	w, err := s.repo.GetWallet(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	per100 := discountPer100Points()
	redeemablePoints := (w.PointsBalance / redeemBlockPoints) * redeemBlockPoints
	redeemableDiscount := (redeemablePoints / redeemBlockPoints) * per100
	return &PointsDiscountResponse{
		UserID:               req.UserID,
		CurrentPoints:        w.PointsBalance,
		RedeemablePoints:     redeemablePoints,
		DiscountPer100Points: per100,
		RedeemableDiscount:   redeemableDiscount,
	}, nil
}

func (s *Service) ApplyDiscount(ctx context.Context, req ApplyDiscountRequest, now int64) (*ApplyDiscountResponse, error) {
	if strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.BillID) == "" {
		return nil, fmt.Errorf("user_id and bill_id required")
	}
	b, err := s.billRepo.GetBill(ctx, req.BillID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("bill not found")
	}
	w, err := s.repo.GetWallet(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if b.LoyaltyUserID != "" && b.LoyaltyUserID != req.UserID {
		return nil, fmt.Errorf("bill already associated with another user")
	}
	// Idempotent if already redeemed on this bill.
	if b.LoyaltyPointsRedeemed > 0 || b.LoyaltyDiscountApplied > 0 {
		return &ApplyDiscountResponse{
			UserID:           req.UserID,
			BillID:           req.BillID,
			PointsRedeemed:   b.LoyaltyPointsRedeemed,
			DiscountApplied:  b.LoyaltyDiscountApplied,
			RemainingPoints:  w.PointsBalance,
			UpdatedBillTotal: b.TotalAmountInPaise,
		}, nil
	}

	per100 := discountPer100Points()
	maxRedeemablePoints := (w.PointsBalance / redeemBlockPoints) * redeemBlockPoints
	if maxRedeemablePoints == 0 {
		return nil, fmt.Errorf("insufficient points to redeem")
	}
	maxDiscountFromPoints := (maxRedeemablePoints / redeemBlockPoints) * per100
	discount := maxDiscountFromPoints
	if discount > b.TotalAmountInPaise {
		discount = (b.TotalAmountInPaise / per100) * per100
	}
	if discount <= 0 {
		return nil, fmt.Errorf("bill total too low for redeem block")
	}
	pointsToRedeem := (discount / per100) * redeemBlockPoints
	if pointsToRedeem <= 0 {
		return nil, fmt.Errorf("invalid redemption points")
	}
	if pointsToRedeem > w.PointsBalance {
		return nil, fmt.Errorf("insufficient points balance")
	}

	w.PointsBalance -= pointsToRedeem
	w.LifetimeRedeemed += pointsToRedeem
	w.UpdatedAt = now
	if err := s.repo.PutWallet(ctx, w); err != nil {
		return nil, err
	}

	b.LoyaltyUserID = req.UserID
	b.LoyaltyPointsRedeemed = pointsToRedeem
	b.LoyaltyDiscountApplied = discount
	b.TotalDiscountInPaise += discount
	if b.TotalAmountInPaise >= discount {
		b.TotalAmountInPaise -= discount
	} else {
		b.TotalAmountInPaise = 0
	}
	applied := false
	for i := range b.Discounts {
		if b.Discounts[i].ID == loyaltyDiscountID {
			b.Discounts[i].Amount = discount
			b.Discounts[i].Description = fmt.Sprintf("%d points redeemed", pointsToRedeem)
			applied = true
			break
		}
	}
	if !applied {
		b.Discounts = append(b.Discounts, billing.DiscountType{
			ID:          loyaltyDiscountID,
			Type:        loyaltyDiscountType,
			Amount:      discount,
			Description: fmt.Sprintf("%d points redeemed", pointsToRedeem),
		})
	}
	b.UpdatedAt = now
	if err := s.billRepo.PutBill(ctx, b); err != nil {
		return nil, err
	}

	return &ApplyDiscountResponse{
		UserID:           req.UserID,
		BillID:           req.BillID,
		PointsRedeemed:   pointsToRedeem,
		DiscountApplied:  discount,
		RemainingPoints:  w.PointsBalance,
		UpdatedBillTotal: b.TotalAmountInPaise,
	}, nil
}
