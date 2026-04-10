package billing

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Clock generates ids and timestamps (implemented by main.CommonUtils).
type Clock interface {
	GenerateUniqueID(prefix *string) string
	GetCurrentTimestamp() int64
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func defaultVenueID() string {
	v := strings.TrimSpace(os.Getenv("TANGIFY_VENUE_ID"))
	if v == "" {
		return "default"
	}
	return v
}

func sumLineItems(items []LineItem) int64 {
	var t int64
	for _, li := range items {
		t += li.Price * int64(li.Quantity)
	}
	return t
}

func ensureLineItemIDs(items []LineItem) []LineItem {
	out := make([]LineItem, len(items))
	for i, li := range items {
		out[i] = li
		if out[i].ID == "" {
			out[i].ID = PrefixLine + "_" + uuid.NewString()
		}
		if out[i].Status == "" {
			out[i].Status = LineItemStatusQueued
		}
	}
	return out
}

func tableInSession(tableIDs []string, tableID string) bool {
	for _, t := range tableIDs {
		if t == tableID {
			return true
		}
	}
	return false
}

func (s *Service) findLiveSessionForTable(ctx context.Context, venueID, tableID string) (*TableSession, error) {
	sessions, err := s.repo.QuerySessionsByVenue(ctx, venueID, 500)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		sess := &sessions[i]
		if sess.Status != SessionStatusLive && sess.Status != SessionStatusBilling {
			continue
		}
		if tableInSession(sess.TableIDs, tableID) {
			return sess, nil
		}
	}
	return nil, nil
}

// --- Waiter ---

func (s *Service) LiveOrdersGrouped(ctx context.Context, venueID string) (*LiveOrdersGroupedResponse, error) {
	if venueID == "" {
		venueID = defaultVenueID()
	}
	sessions, err := s.repo.QuerySessionsByVenue(ctx, venueID, 500)
	if err != nil {
		return nil, err
	}
	var bundles []SessionWithOrders
	for i := range sessions {
		sess := sessions[i]
		if sess.Status != SessionStatusLive && sess.Status != SessionStatusBilling {
			continue
		}
		orders, err := s.repo.QueryOrdersBySession(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, SessionWithOrders{Session: sess, Orders: orders})
	}
	return &LiveOrdersGroupedResponse{Sessions: bundles}, nil
}

func (s *Service) CreateSessionAndFirstOrder(ctx context.Context, req CreateSessionAndFirstOrderRequest, staffID string, clock Clock) (*SessionWithOrders, error) {
	if len(req.TableIDs) == 0 {
		return nil, fmt.Errorf("table_ids required")
	}
	if req.Channel == "" {
		return nil, fmt.Errorf("channel required")
	}
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("items required")
	}
	venueID := defaultVenueID()
	now := clock.GetCurrentTimestamp()
	pSess := PrefixSession
	sid := clock.GenerateUniqueID(&pSess)
	pOrd := PrefixOrder
	oid := clock.GenerateUniqueID(&pOrd)

	items := ensureLineItemIDs(req.Items)
	cust := ""
	if req.CustomerID != nil {
		cust = *req.CustomerID
	}
	st := staffID
	if req.StaffID != nil && *req.StaffID != "" {
		st = *req.StaffID
	}
	orderedAt := now
	if req.OrderedAt != nil && *req.OrderedAt != 0 {
		orderedAt = *req.OrderedAt
	}

	session := TableSession{
		ID:        sid,
		TableIDs:  req.TableIDs,
		Status:    SessionStatusLive,
		OpenedAt:  now,
		UpdatedAt: now,
		VenueID:   venueID,
	}
	order := Order{
		ID:            oid,
		SessionID:     sid,
		VenueID:       venueID,
		Channel:       req.Channel,
		CustomerID:    cust,
		StaffID:       st,
		Items:         items,
		TotalPrice:    sumLineItems(items),
		KitchenStatus: KitchenStatusPending,
		OrderedAt:     orderedAt,
		UpdatedAt:     now,
	}
	if err := s.repo.PutSession(ctx, &session); err != nil {
		return nil, err
	}
	if err := s.repo.PutOrder(ctx, &order); err != nil {
		return nil, err
	}
	return &SessionWithOrders{Session: session, Orders: []Order{order}}, nil
}

func (s *Service) AddOrder(ctx context.Context, req AddOrderToSessionRequest, staffID string, clock Clock) (*Order, error) {
	if req.SessionID == "" || req.Channel == "" || len(req.Items) == 0 {
		return nil, fmt.Errorf("session_id, channel, and items required")
	}
	sess, err := s.repo.GetSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found")
	}
	if sess.Status != SessionStatusLive && sess.Status != SessionStatusBilling {
		return nil, fmt.Errorf("session is not open for orders")
	}
	venueID := sess.VenueID
	if venueID == "" {
		venueID = defaultVenueID()
	}
	now := clock.GetCurrentTimestamp()
	pOrd := PrefixOrder
	oid := clock.GenerateUniqueID(&pOrd)
	items := ensureLineItemIDs(req.Items)
	st := staffID
	if req.StaffID != nil && *req.StaffID != "" {
		st = *req.StaffID
	}
	orderedAt := now
	if req.OrderedAt != nil && *req.OrderedAt != 0 {
		orderedAt = *req.OrderedAt
	}
	src := ""
	if req.SourceTableID != nil {
		src = *req.SourceTableID
	}
	order := Order{
		ID:            oid,
		SessionID:     req.SessionID,
		VenueID:       venueID,
		Channel:       req.Channel,
		SourceTableID: src,
		StaffID:       st,
		Items:         items,
		TotalPrice:    sumLineItems(items),
		KitchenStatus: KitchenStatusPending,
		OrderedAt:     orderedAt,
		UpdatedAt:     now,
	}
	if sess.BillID != "" {
		order.BillID = sess.BillID
	}
	if err := s.repo.PutOrder(ctx, &order); err != nil {
		return nil, err
	}
	sess.UpdatedAt = now
	if err := s.repo.PutSession(ctx, sess); err != nil {
		return nil, err
	}
	return &order, nil
}

// UpdateOrderWithClock updates items / kitchen status on an order.
func (s *Service) UpdateOrderWithClock(ctx context.Context, req UpdateOrderRequestV2, clock Clock) (*Order, error) {
	o, err := s.repo.GetOrder(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("order not found")
	}
	if len(req.Items) > 0 {
		o.Items = ensureLineItemIDs(req.Items)
		o.TotalPrice = sumLineItems(o.Items)
	}
	if req.KitchenStatus != nil {
		o.KitchenStatus = *req.KitchenStatus
	}
	if req.TotalPrice != nil {
		o.TotalPrice = *req.TotalPrice
	}
	o.UpdatedAt = clock.GetCurrentTimestamp()
	if err := s.repo.PutOrder(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Service) ListOrdersBySession(ctx context.Context, sessionID string) ([]Order, error) {
	return s.repo.QueryOrdersBySession(ctx, sessionID)
}

func (s *Service) ListOrdersByTable(ctx context.Context, venueID, tableID string) ([]Order, error) {
	if venueID == "" {
		venueID = defaultVenueID()
	}
	sess, err := s.findLiveSessionForTable(ctx, venueID, tableID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return []Order{}, nil
	}
	return s.repo.QueryOrdersBySession(ctx, sess.ID)
}

func (s *Service) StartBill(ctx context.Context, req StartBillForSessionRequest, staffID string, clock Clock) (*Bill, error) {
	sess, err := s.repo.GetSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found")
	}
	if sess.Status != SessionStatusLive {
		return nil, fmt.Errorf("session must be live to start bill")
	}
	if sess.BillID != "" {
		b, err := s.repo.GetBill(ctx, sess.BillID)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	now := clock.GetCurrentTimestamp()
	pB := PrefixBill
	bid := clock.GenerateUniqueID(&pB)
	st := staffID
	if req.StaffID != nil && *req.StaffID != "" {
		st = *req.StaffID
	}
	bill := Bill{
		ID:                   bid,
		SessionID:            sess.ID,
		TableIDs:             append([]string(nil), sess.TableIDs...),
		StaffID:              st,
		PaymentMethod:        PaymentMethodCash,
		PaymentStatus:        PaymentStatusPending,
		CreatedAt:            now,
		UpdatedAt:            now,
		Discounts:            nil,
		Taxes:                nil,
		TotalTaxInPaise:      0,
		TotalDiscountInPaise: 0,
		TotalAmountInPaise:   0,
	}
	sess.BillID = bid
	sess.Status = SessionStatusBilling
	sess.UpdatedAt = now
	if err := s.repo.PutBill(ctx, &bill); err != nil {
		return nil, err
	}
	if err := s.repo.PutSession(ctx, sess); err != nil {
		return nil, err
	}
	orders, err := s.repo.QueryOrdersBySession(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	var total int64
	for i := range orders {
		orders[i].BillID = bid
		total += orders[i].TotalPrice
		if err := s.repo.PutOrder(ctx, &orders[i]); err != nil {
			return nil, err
		}
	}
	bill.TotalAmountInPaise = total
	bill.UpdatedAt = clock.GetCurrentTimestamp()
	if err := s.repo.PutBill(ctx, &bill); err != nil {
		return nil, err
	}
	return &bill, nil
}

func (s *Service) UpdateBill(ctx context.Context, req UpdateBillRequestV2, clock Clock) (*Bill, error) {
	b, err := s.repo.GetBill(ctx, req.BillID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("bill not found")
	}
	if len(req.Discounts) > 0 {
		b.Discounts = req.Discounts
	}
	if len(req.Taxes) > 0 {
		b.Taxes = req.Taxes
	}
	if req.PaymentMethod != nil {
		b.PaymentMethod = *req.PaymentMethod
	}
	if req.PaymentStatus != nil {
		b.PaymentStatus = *req.PaymentStatus
	}
	if req.TotalAmountInPaise != nil {
		b.TotalAmountInPaise = *req.TotalAmountInPaise
	}
	b.UpdatedAt = clock.GetCurrentTimestamp()
	if err := s.repo.PutBill(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) CloseTable(ctx context.Context, req CloseTableRequest, clock Clock) error {
	sess, err := s.repo.GetSession(ctx, req.SessionID)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("session not found")
	}
	if req.BillID == "" || sess.BillID != req.BillID {
		return fmt.Errorf("bill_id mismatch")
	}
	b, err := s.repo.GetBill(ctx, req.BillID)
	if err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("bill not found")
	}
	now := clock.GetCurrentTimestamp()
	b.PaymentStatus = PaymentStatusPaid
	b.UpdatedAt = now
	sess.Status = SessionStatusClosed
	sess.ClosedAt = now
	sess.UpdatedAt = now
	if err := s.repo.PutBill(ctx, b); err != nil {
		return err
	}
	return s.repo.PutSession(ctx, sess)
}

// --- Kitchen ---

func (s *Service) KitchenItemBoard(ctx context.Context, venueID string) ([]KitchenDishCount, error) {
	if venueID == "" {
		venueID = defaultVenueID()
	}
	orders, err := s.repo.QueryOrdersByVenue(ctx, venueID, 500)
	if err != nil {
		return nil, err
	}
	var rows []KitchenDishCount
	for _, o := range orders {
		for _, li := range o.Items {
			if li.Status == LineItemStatusServed || li.Status == LineItemStatusCancelled {
				continue
			}
			rows = append(rows, KitchenDishCount{
				OrderID:    o.ID,
				LineItemID: li.ID,
				Name:       li.Name,
				Quantity:   li.Quantity,
				Status:     li.Status,
			})
		}
	}
	return rows, nil
}

func (s *Service) PatchLineItemStatus(ctx context.Context, req PatchLineItemStatusRequest, clock Clock) (*Order, error) {
	o, err := s.repo.GetOrder(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("order not found")
	}
	found := false
	for i := range o.Items {
		if o.Items[i].ID == req.LineItemID {
			o.Items[i].Status = req.Status
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("line item not found")
	}
	o.UpdatedAt = clock.GetCurrentTimestamp()
	if err := s.repo.PutOrder(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// --- Plating ---

func (s *Service) PlatingFIFO(ctx context.Context, venueID, tableID, sessionID string, limit int) ([]PlatingQueueOrder, error) {
	if venueID == "" {
		venueID = defaultVenueID()
	}
	if limit <= 0 {
		limit = 100
	}
	var orders []Order
	var err error
	if sessionID != "" {
		orders, err = s.repo.QueryOrdersBySession(ctx, sessionID)
	} else if tableID != "" {
		sess, e := s.findLiveSessionForTable(ctx, venueID, tableID)
		if e != nil {
			return nil, e
		}
		if sess == nil {
			return nil, nil
		}
		orders, err = s.repo.QueryOrdersBySession(ctx, sess.ID)
	} else {
		return nil, fmt.Errorf("session_id or table_id required")
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].OrderedAt < orders[j].OrderedAt })
	if len(orders) > limit {
		orders = orders[:limit]
	}
	var sess *TableSession
	if len(orders) > 0 {
		sess, _ = s.repo.GetSession(ctx, orders[0].SessionID)
	}
	var tids []string
	if sess != nil {
		tids = sess.TableIDs
	}
	out := make([]PlatingQueueOrder, 0, len(orders))
	for _, o := range orders {
		if o.KitchenStatus == KitchenStatusServed {
			continue
		}
		out = append(out, PlatingQueueOrder{
			OrderID:       o.ID,
			SessionID:     o.SessionID,
			TableIDs:      tids,
			KitchenStatus: o.KitchenStatus,
			OrderedAt:     o.OrderedAt,
		})
	}
	return out, nil
}

func (s *Service) PatchOrderKitchenStatus(ctx context.Context, req PatchOrderKitchenStatusRequestV2, clock Clock) (*Order, error) {
	o, err := s.repo.GetOrder(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("order not found")
	}
	o.KitchenStatus = req.KitchenStatus
	now := clock.GetCurrentTimestamp()
	o.UpdatedAt = now
	if req.KitchenStatus == KitchenStatusReady {
		o.ReadyAt = now
	}
	if req.KitchenStatus == KitchenStatusServed {
		o.CompletedAt = now
	}
	if err := s.repo.PutOrder(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// Remove duplicate broken UpdateOrder - delete the first UpdateOrder that returns error
