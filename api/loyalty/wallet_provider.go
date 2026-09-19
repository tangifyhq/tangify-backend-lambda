package loyalty

import (
	"context"
	"fmt"
	"strings"

	"tangify-backend-lambda/billing"
	"tangify-backend-lambda/users"
)

// WalletView is the staff wallet lookup payload.
type WalletView struct {
	Phone         string `json:"phone"`
	UserID        string `json:"user_id"`
	PointsBalance int64  `json:"points_balance"`
}

type customerUsers interface {
	CreateOrGetCustomer(ctx context.Context, phone, name string, now int64) (*users.UserPublic, error)
}

// WalletProvider implements billing.PointsWalletProvider.
type WalletProvider struct {
	repo  *Repository
	users customerUsers
}

func NewWalletProvider(repo *Repository, users customerUsers) *WalletProvider {
	return &WalletProvider{repo: repo, users: users}
}

func (p *WalletProvider) GetPointsBalance(ctx context.Context, userID string) (int64, error) {
	w, err := p.repo.GetWallet(ctx, userID)
	if err != nil {
		return 0, err
	}
	return w.PointsBalance, nil
}

// WebWalletResponse is the customer web ordering loyalty balance payload.
type WebWalletResponse struct {
	UserID                 string `json:"user_id"`
	PointsBalance          int64  `json:"points_balance"`
	EarnSpendPaisePerPoint int64  `json:"earn_spend_paise_per_point"`
	RedeemPaisePerPoint    int64  `json:"redeem_paise_per_point"`
}

func (p *WalletProvider) GetWebWallet(ctx context.Context, userID string) (*WebWalletResponse, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user_id required")
	}
	bal, err := p.GetPointsBalance(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &WebWalletResponse{
		UserID:                 userID,
		PointsBalance:          bal,
		EarnSpendPaisePerPoint: earnPointsPerSpendPaise,
		RedeemPaisePerPoint:    discountPer100Points(),
	}, nil
}

func (p *WalletProvider) ResolvePhone(ctx context.Context, phone string, now int64) (*billing.ResolvedLoyaltyCustomer, error) {
	view, err := p.GetOrCreateByPhone(ctx, phone, now)
	if err != nil {
		return nil, err
	}
	return &billing.ResolvedLoyaltyCustomer{
		UserID:        view.UserID,
		Phone:         view.Phone,
		PointsBalance: view.PointsBalance,
	}, nil
}

func (p *WalletProvider) GetOrCreateByPhone(ctx context.Context, phone string, now int64) (*WalletView, error) {
	canon, err := users.CanonicalPhone(phone)
	if err != nil {
		return nil, err
	}
	user, err := p.users.CreateOrGetCustomer(ctx, canon, "", now)
	if err != nil {
		return nil, err
	}
	w, err := p.repo.GetWallet(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if w.UpdatedAt == 0 && w.PointsBalance == 0 && w.LifetimeEarned == 0 && w.LifetimeRedeemed == 0 {
		w.UserID = user.ID
		w.UpdatedAt = now
		if err := p.repo.PutWallet(ctx, w); err != nil {
			return nil, err
		}
	}
	return &WalletView{
		Phone:         canon,
		UserID:        user.ID,
		PointsBalance: w.PointsBalance,
	}, nil
}
