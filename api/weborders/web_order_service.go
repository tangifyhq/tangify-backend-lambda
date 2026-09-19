package weborders

import (
	"context"
	"fmt"
	"strings"
)

type WebOrderService struct {
	repo *WebOrderRepository
	pay  *RazorpayService
}

func NewWebOrderService(repo *WebOrderRepository, pay *RazorpayService) *WebOrderService {
	return &WebOrderService{repo: repo, pay: pay}
}

func (s *WebOrderService) VerifyAndSave(
	ctx context.Context,
	cfg RazorpayConfig,
	userID string,
	req VerifyRazorpayPaymentRequest,
	nowUnix int64,
) (*VerifyRazorpayPaymentResponse, error) {
	if s == nil || s.pay == nil || s.repo == nil {
		return nil, fmt.Errorf("web order service is not configured")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user required")
	}
	if err := req.Order.Validate(); err != nil {
		return nil, err
	}

	verified, err := s.pay.VerifyPayment(cfg, req)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByRef(ctx, req.Order.OrderRef)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.UserID != userID {
			return nil, fmt.Errorf("order_ref already used")
		}
		if existing.PaymentID != "" &&
			existing.PaymentID != strings.TrimSpace(req.RazorpayPaymentID) {
			return nil, fmt.Errorf("order_ref already paid with a different payment")
		}
		return &VerifyRazorpayPaymentResponse{
			Verified:  true,
			OrderID:   verified.OrderID,
			PaymentID: verified.PaymentID,
			Order:     existing,
		}, nil
	}

	order := BuildWebOrder(userID, verified.PaymentID, verified.OrderID, req.Order, nowUnix)
	created, err := s.repo.PutIfAbsent(ctx, order)
	if err != nil {
		return nil, err
	}
	if !created {
		existing, err = s.repo.GetByRef(ctx, order.OrderRef)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, fmt.Errorf("failed to persist order")
		}
		if existing.UserID != userID {
			return nil, fmt.Errorf("order_ref already used")
		}
		order = existing
	}

	return &VerifyRazorpayPaymentResponse{
		Verified:  true,
		OrderID:   verified.OrderID,
		PaymentID: verified.PaymentID,
		Order:     order,
	}, nil
}

func (s *WebOrderService) GetByRef(ctx context.Context, orderRef string) (*WebOrder, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("web order service is not configured")
	}
	return s.repo.GetByRef(ctx, orderRef)
}

func (s *WebOrderService) ListForUser(ctx context.Context, userID string) ([]WebOrder, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("web order service is not configured")
	}
	return s.repo.ListByUser(ctx, userID, 50)
}
