package billing

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"tangify-backend-lambda/users"
)

type LoyaltyNotifier interface {
	NotifyWalletSummary(ctx context.Context, phone string, redeem, earn, balance int64)
}

type BillWithLineItemsService struct {
	repo              *BillWithLineItemsRepository
	wallet            PointsWalletProvider
	invoiceWorkerURL  string
	notifier          LoyaltyNotifier
	pointsWalletTable string
}

func NewBillWithLineItemsService(
	repo *BillWithLineItemsRepository,
	wallet PointsWalletProvider,
	invoiceWorkerURL string,
	notifier LoyaltyNotifier,
	pointsWalletTable string,
) *BillWithLineItemsService {
	table := strings.TrimSpace(pointsWalletTable)
	if table == "" {
		table = "tangify_points_wallet"
	}
	return &BillWithLineItemsService{
		repo:              repo,
		wallet:            wallet,
		invoiceWorkerURL:  strings.TrimSpace(invoiceWorkerURL),
		notifier:          notifier,
		pointsWalletTable: table,
	}
}

func (s *BillWithLineItemsService) Get(ctx context.Context, billID string) (*BillWithLineItems, error) {
	billID = strings.TrimSpace(billID)
	if billID == "" {
		return nil, fmt.Errorf("bill_id required")
	}
	return s.repo.Get(ctx, billID)
}

func (s *BillWithLineItemsService) Upsert(
	ctx context.Context,
	req UpsertBillWithLineItemsRequest,
	staffID string,
	now int64,
) (*BillWithLineItems, error) {
	id := strings.TrimSpace(req.ID)
	stateKey := strings.TrimSpace(req.StateKey)

	if id != "" {
		return s.update(ctx, req, id, staffID, now)
	}
	if stateKey == "" {
		return nil, errStateKeyRequired
	}
	return s.create(ctx, req, stateKey, staffID, now)
}

func invoiceWorkerBillID(stateKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(stateKey)))
	return fmt.Sprintf("%s_state_%x", PrefixBill, sum[:16])
}

func linkedLoyaltyCustomerID(req UpsertBillWithLineItemsRequest) string {
	return strings.TrimSpace(req.LoyaltyCustomerID)
}

func (s *BillWithLineItemsService) resolveCustomer(
	ctx context.Context,
	raw string,
	now int64,
) (userID, phone string, balance int64, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", 0, nil
	}
	if s.wallet == nil {
		return raw, raw, 0, nil
	}
	if _, canonErr := users.CanonicalPhone(raw); canonErr == nil {
		resolved, resErr := s.wallet.ResolvePhone(ctx, raw, now)
		if resErr != nil {
			return "", "", 0, resErr
		}
		return resolved.UserID, resolved.Phone, resolved.PointsBalance, nil
	}
	bal, balErr := s.wallet.GetPointsBalance(ctx, raw)
	if balErr != nil {
		return "", "", 0, balErr
	}
	return raw, raw, bal, nil
}

func (s *BillWithLineItemsService) create(
	ctx context.Context,
	req UpsertBillWithLineItemsRequest,
	stateKey string,
	staffID string,
	now int64,
) (*BillWithLineItems, error) {
	existing, err := s.repo.GetByStateKey(ctx, stateKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	userID, phone, pointsBalance, err := s.resolveCustomer(ctx, linkedLoyaltyCustomerID(req), now)
	if err != nil {
		return nil, err
	}
	billCustomerID := strings.TrimSpace(req.CustomerID)
	if billCustomerID == "" && linkedLoyaltyCustomerID(req) != "" && phone != "" {
		billCustomerID = phone
	}

	totals, err := computeBillTotals(
		req.LineItems,
		req.Discounts,
		req.Taxes,
		phone,
		pointsBalance,
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}

	workerBillID := invoiceWorkerBillID(stateKey)
	inv, err := FetchInvoiceNumberWithURL(ctx, workerBillID, s.invoiceWorkerURL)
	if err != nil {
		return nil, err
	}

	bill := requestToBill(req, staffID, now)
	bill.ID = inv.InvoiceNumber
	bill.StateKey = stateKey
	bill.CreatedAt = now
	bill.UpdatedAt = now
	bill.Discounts = totals.Discounts
	bill.Taxes = totals.Taxes
	bill.TotalDiscountInPaise = totals.TotalDiscountInPaise
	bill.TotalTaxInPaise = totals.TotalTaxInPaise
	bill.TotalAmountInPaise = totals.TotalAmountInPaise
	if billCustomerID != "" {
		bill.CustomerID = billCustomerID
	}
	if bill.PaymentMethod == "" {
		bill.PaymentMethod = PaymentMethodCash
	}
	if bill.PaymentStatus == "" {
		bill.PaymentStatus = PaymentStatusPending
	}

	redeem := totals.PointsRedeemed
	var earn int64
	if req.Settled && linkedLoyaltyCustomerID(req) != "" && phone != "" {
		earn = PointsEarnedFromDiscountedSubtotal(
			lineItemsSubtotalPaise(req.LineItems),
			totals.TotalDiscountInPaise,
		)
		bill.Settled = true
		bill.SettledAt = now
		bill.LoyaltyPointsProcessed = true
		bill.LoyaltyPointsEarned = earn
	}
	if redeem > 0 {
		bill.LoyaltyPointsRedeemed = redeem
	}

	err = s.repo.TransactWrite(
		ctx,
		bill,
		userID,
		redeem,
		earn,
		s.pointsWalletTable,
		now,
		true,
	)
	if err != nil {
		existing, getErr := s.repo.Get(ctx, inv.InvoiceNumber)
		if getErr == nil && existing != nil && existing.StateKey == stateKey {
			return existing, nil
		}
		return nil, err
	}
	s.notifyWallet(ctx, phone, pointsBalance, redeem, earn)
	return bill, nil
}

func (s *BillWithLineItemsService) update(
	ctx context.Context,
	req UpsertBillWithLineItemsRequest,
	id string,
	staffID string,
	now int64,
) (*BillWithLineItems, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrBillNotFound
	}

	loyaltyPhone := linkedLoyaltyCustomerID(req)
	billCustomerID := strings.TrimSpace(req.CustomerID)
	if billCustomerID == "" {
		billCustomerID = strings.TrimSpace(existing.CustomerID)
	}
	if billCustomerID == "" && loyaltyPhone != "" {
		billCustomerID = loyaltyPhone
	}

	totals, err := computeBillTotals(
		req.LineItems,
		req.Discounts,
		req.Taxes,
		loyaltyPhone,
		0,
		true,
		existing.Discounts,
	)
	if err != nil {
		return nil, err
	}

	bill := requestToBill(req, staffID, now)
	bill.ID = id
	bill.StateKey = existing.StateKey
	bill.CreatedAt = existing.CreatedAt
	bill.UpdatedAt = now
	bill.Discounts = totals.Discounts
	bill.Taxes = totals.Taxes
	bill.TotalDiscountInPaise = totals.TotalDiscountInPaise
	bill.TotalTaxInPaise = totals.TotalTaxInPaise
	bill.TotalAmountInPaise = totals.TotalAmountInPaise
	bill.Settled = existing.Settled
	bill.SettledAt = existing.SettledAt
	bill.LoyaltyPointsProcessed = existing.LoyaltyPointsProcessed
	bill.LoyaltyPointsEarned = existing.LoyaltyPointsEarned
	bill.LoyaltyPointsRedeemed = existing.LoyaltyPointsRedeemed
	if bill.PaymentMethod == "" {
		bill.PaymentMethod = existing.PaymentMethod
	}
	if bill.PaymentStatus == "" {
		bill.PaymentStatus = existing.PaymentStatus
	}
	if billCustomerID != "" {
		bill.CustomerID = billCustomerID
	}
	if bill.SessionID == "" {
		bill.SessionID = existing.SessionID
	}
	if len(bill.TableIDs) == 0 {
		bill.TableIDs = existing.TableIDs
	}

	shouldEarn := req.Settled && !existing.LoyaltyPointsProcessed && loyaltyPhone != ""
	if shouldEarn {
		userID, phone, balance, resErr := s.resolveCustomer(ctx, loyaltyPhone, now)
		if resErr != nil {
			return nil, resErr
		}
		earn := PointsEarnedFromDiscountedSubtotal(
			lineItemsSubtotalPaise(req.LineItems),
			totals.TotalDiscountInPaise,
		)
		bill.Settled = true
		if bill.SettledAt == 0 {
			bill.SettledAt = now
		}
		bill.LoyaltyPointsProcessed = true
		bill.LoyaltyPointsEarned = earn
		if err := s.repo.TransactWrite(
			ctx,
			bill,
			userID,
			0,
			earn,
			s.pointsWalletTable,
			now,
			false,
		); err != nil {
			latest, getErr := s.repo.Get(ctx, id)
			if getErr == nil && latest != nil && latest.LoyaltyPointsProcessed {
				return latest, nil
			}
			return nil, err
		}
		s.notifyWallet(ctx, phone, balance, 0, earn)
		return bill, nil
	}

	if req.Settled {
		bill.Settled = true
		if bill.SettledAt == 0 {
			bill.SettledAt = now
		}
	}

	if err := s.repo.Put(ctx, bill); err != nil {
		return nil, err
	}
	return bill, nil
}

func (s *BillWithLineItemsService) notifyWallet(ctx context.Context, phone string, startBalance, redeem, earn int64) {
	if s.notifier == nil || strings.TrimSpace(phone) == "" {
		return
	}
	if redeem == 0 && earn == 0 {
		return
	}
	finalBalance := startBalance - redeem + earn
	s.notifier.NotifyWalletSummary(ctx, phone, redeem, earn, finalBalance)
}

func requestToBill(req UpsertBillWithLineItemsRequest, staffID string, now int64) *BillWithLineItems {
	bill := &BillWithLineItems{
		SessionID:  req.SessionID,
		TableIDs:   append([]string(nil), req.TableIDs...),
		CustomerID: strings.TrimSpace(req.CustomerID),
		LineItems:  append([]LineItemV0(nil), req.LineItems...),
		Discounts:  append([]DiscountType(nil), req.Discounts...),
		Taxes:      append([]TaxType(nil), req.Taxes...),
		UpdatedAt:  now,
	}
	if strings.TrimSpace(req.StaffID) != "" {
		bill.StaffID = strings.TrimSpace(req.StaffID)
	} else {
		bill.StaffID = staffID
	}
	if req.PaymentMethod != nil {
		bill.PaymentMethod = *req.PaymentMethod
	}
	if req.PaymentStatus != nil {
		bill.PaymentStatus = *req.PaymentStatus
	}
	return bill
}
