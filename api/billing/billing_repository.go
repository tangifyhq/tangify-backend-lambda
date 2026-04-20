package billing

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Repository struct {
	db *dynamodb.Client
}

func NewRepository(db *dynamodb.Client) *Repository {
	return &Repository{db: db}
}

// --- Session ---

func (r *Repository) PutSession(ctx context.Context, s *TableSession) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	item, err := encodeSession(s)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNameSessions),
		Item:      item,
	})
	return err
}

func (r *Repository) GetSession(ctx context.Context, id string) (*TableSession, error) {
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNameSessions),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return decodeSession(out.Item)
}

// QuerySessionsByVenue returns sessions for a venue (newest first by opened_at via GSI).
func (r *Repository) QuerySessionsByVenue(ctx context.Context, venueID string, limit int32) ([]TableSession, error) {
	if limit <= 0 {
		limit = 200
	}
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameSessions),
		IndexName:              aws.String(GSIVenueOpened),
		KeyConditionExpression: aws.String("venue_id = :v"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":v": &types.AttributeValueMemberS{Value: venueID},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(limit),
	})
	if err != nil {
		return nil, err
	}
	var sessions []TableSession
	for _, it := range out.Items {
		s, err := decodeSession(it)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
	}
	return sessions, nil
}

// --- Order ---

func (r *Repository) PutOrder(ctx context.Context, o *Order) error {
	if o == nil {
		return fmt.Errorf("order is nil")
	}
	item, err := encodeOrder(o)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNameOrders),
		Item:      item,
	})
	return err
}

func (r *Repository) GetOrder(ctx context.Context, id string) (*Order, error) {
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNameOrders),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return decodeOrder(out.Item)
}

func (r *Repository) QueryOrdersBySession(ctx context.Context, sessionID string) ([]Order, error) {
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameOrders),
		IndexName:              aws.String(GSISessionOrdered),
		KeyConditionExpression: aws.String("session_id = :s"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: sessionID},
		},
		ScanIndexForward: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	return decodeOrderList(out.Items)
}

func (r *Repository) QueryOrdersByVenue(ctx context.Context, venueID string, limit int32) ([]Order, error) {
	if limit <= 0 {
		limit = 500
	}
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameOrders),
		IndexName:              aws.String(GSIVenueOrdered),
		KeyConditionExpression: aws.String("venue_id = :v"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":v": &types.AttributeValueMemberS{Value: venueID},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return decodeOrderList(out.Items)
}

// --- Bill ---

func (r *Repository) PutBill(ctx context.Context, b *Bill) error {
	if b == nil {
		return fmt.Errorf("bill is nil")
	}
	item, err := encodeBill(b)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNameBills),
		Item:      item,
	})
	return err
}

func (r *Repository) GetBill(ctx context.Context, id string) (*Bill, error) {
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNameBills),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return decodeBill(out.Item)
}

func (r *Repository) QueryBillsBySession(ctx context.Context, sessionID string) ([]Bill, error) {
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameBills),
		IndexName:              aws.String(GSISessionBill),
		KeyConditionExpression: aws.String("session_id = :s"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: sessionID},
		},
	})
	if err != nil {
		return nil, err
	}
	var bills []Bill
	for _, it := range out.Items {
		b, err := decodeBill(it)
		if err != nil {
			return nil, err
		}
		bills = append(bills, *b)
	}
	return bills, nil
}

// --- encode / decode ---

func encodeSession(s *TableSession) (map[string]types.AttributeValue, error) {
	tids := make([]types.AttributeValue, 0, len(s.TableIDs))
	for _, t := range s.TableIDs {
		tids = append(tids, &types.AttributeValueMemberS{Value: t})
	}
	m := map[string]types.AttributeValue{
		"id":        &types.AttributeValueMemberS{Value: s.ID},
		"venue_id":  &types.AttributeValueMemberS{Value: s.VenueID},
		"opened_at": &types.AttributeValueMemberN{Value: strconv.FormatInt(s.OpenedAt, 10)},
		"status":    &types.AttributeValueMemberS{Value: s.Status},
		"table_ids": &types.AttributeValueMemberL{Value: tids},
	}
	if s.BillID != "" {
		m["bill_id"] = &types.AttributeValueMemberS{Value: s.BillID}
	}
	if s.ClosedAt != 0 {
		m["closed_at"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(s.ClosedAt, 10)}
	}
	if s.UpdatedAt != 0 {
		m["updated_at"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(s.UpdatedAt, 10)}
	}
	return m, nil
}

func decodeSession(item map[string]types.AttributeValue) (*TableSession, error) {
	s := &TableSession{}
	if v, ok := item["id"].(*types.AttributeValueMemberS); ok {
		s.ID = v.Value
	}
	if v, ok := item["venue_id"].(*types.AttributeValueMemberS); ok {
		s.VenueID = v.Value
	}
	if v, ok := item["status"].(*types.AttributeValueMemberS); ok {
		s.Status = v.Value
	}
	if v, ok := item["bill_id"].(*types.AttributeValueMemberS); ok {
		s.BillID = v.Value
	}
	s.OpenedAt, _ = numAttr(item, "opened_at")
	s.ClosedAt, _ = numAttr(item, "closed_at")
	s.UpdatedAt, _ = numAttr(item, "updated_at")
	if l, ok := item["table_ids"].(*types.AttributeValueMemberL); ok {
		for _, e := range l.Value {
			if sv, ok := e.(*types.AttributeValueMemberS); ok {
				s.TableIDs = append(s.TableIDs, sv.Value)
			}
		}
	}
	return s, nil
}

func encodeOrder(o *Order) (map[string]types.AttributeValue, error) {
	items := make([]types.AttributeValue, 0, len(o.Items))
	for _, li := range o.Items {
		im := map[string]types.AttributeValue{
			"id":       &types.AttributeValueMemberS{Value: li.ID},
			"name":     &types.AttributeValueMemberS{Value: li.Name},
			"quantity": &types.AttributeValueMemberN{Value: strconv.Itoa(li.Quantity)},
			"price":    &types.AttributeValueMemberN{Value: strconv.FormatInt(li.Price, 10)},
			"status":   &types.AttributeValueMemberS{Value: li.Status},
		}
		if li.UserOverrride != nil {
			im["user_overrride"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(*li.UserOverrride, 10)}
		}
		if li.Removed {
			im["removed"] = &types.AttributeValueMemberBOOL{Value: true}
		}
		items = append(items, &types.AttributeValueMemberM{Value: im})
	}
	m := map[string]types.AttributeValue{
		"id":             &types.AttributeValueMemberS{Value: o.ID},
		"session_id":     &types.AttributeValueMemberS{Value: o.SessionID},
		"venue_id":       &types.AttributeValueMemberS{Value: o.VenueID},
		"ordered_at":     &types.AttributeValueMemberN{Value: strconv.FormatInt(o.OrderedAt, 10)},
		"channel":        &types.AttributeValueMemberS{Value: o.Channel},
		"items":          &types.AttributeValueMemberL{Value: items},
		"total_price":    &types.AttributeValueMemberN{Value: strconv.FormatInt(o.TotalPrice, 10)},
		"kitchen_status": &types.AttributeValueMemberS{Value: o.KitchenStatus},
	}
	if o.BillID != "" {
		m["bill_id"] = &types.AttributeValueMemberS{Value: o.BillID}
	}
	if o.SourceTableID != "" {
		m["source_table_id"] = &types.AttributeValueMemberS{Value: o.SourceTableID}
	}
	if o.CustomerID != "" {
		m["customer_id"] = &types.AttributeValueMemberS{Value: o.CustomerID}
	}
	if o.StaffID != "" {
		m["staff_id"] = &types.AttributeValueMemberS{Value: o.StaffID}
	}
	if o.ReadyAt != 0 {
		m["ready_at"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(o.ReadyAt, 10)}
	}
	if o.CompletedAt != 0 {
		m["completed_at"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(o.CompletedAt, 10)}
	}
	if o.UpdatedAt != 0 {
		m["updated_at"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(o.UpdatedAt, 10)}
	}
	return m, nil
}

func decodeOrder(item map[string]types.AttributeValue) (*Order, error) {
	o := &Order{}
	if v, ok := item["id"].(*types.AttributeValueMemberS); ok {
		o.ID = v.Value
	}
	if v, ok := item["session_id"].(*types.AttributeValueMemberS); ok {
		o.SessionID = v.Value
	}
	if v, ok := item["venue_id"].(*types.AttributeValueMemberS); ok {
		o.VenueID = v.Value
	}
	if v, ok := item["channel"].(*types.AttributeValueMemberS); ok {
		o.Channel = v.Value
	}
	if v, ok := item["bill_id"].(*types.AttributeValueMemberS); ok {
		o.BillID = v.Value
	}
	if v, ok := item["source_table_id"].(*types.AttributeValueMemberS); ok {
		o.SourceTableID = v.Value
	}
	if v, ok := item["customer_id"].(*types.AttributeValueMemberS); ok {
		o.CustomerID = v.Value
	}
	if v, ok := item["staff_id"].(*types.AttributeValueMemberS); ok {
		o.StaffID = v.Value
	}
	if v, ok := item["kitchen_status"].(*types.AttributeValueMemberS); ok {
		o.KitchenStatus = v.Value
	}
	o.TotalPrice, _ = numAttr(item, "total_price")
	o.OrderedAt, _ = numAttr(item, "ordered_at")
	o.ReadyAt, _ = numAttr(item, "ready_at")
	o.CompletedAt, _ = numAttr(item, "completed_at")
	o.UpdatedAt, _ = numAttr(item, "updated_at")
	if l, ok := item["items"].(*types.AttributeValueMemberL); ok {
		for _, e := range l.Value {
			m, ok := e.(*types.AttributeValueMemberM)
			if !ok {
				continue
			}
			li := LineItem{}
			if s, ok := m.Value["id"].(*types.AttributeValueMemberS); ok {
				li.ID = s.Value
			}
			if s, ok := m.Value["name"].(*types.AttributeValueMemberS); ok {
				li.Name = s.Value
			}
			li.Quantity, _ = atoiAttr(m.Value, "quantity")
			li.Price, _ = numAttr(m.Value, "price")
			if n, ok := m.Value["user_overrride"].(*types.AttributeValueMemberN); ok {
				if p, err := strconv.ParseInt(n.Value, 10, 64); err == nil {
					li.UserOverrride = &p
				}
			}
			if b, ok := m.Value["removed"].(*types.AttributeValueMemberBOOL); ok {
				li.Removed = b.Value
			}
			if s, ok := m.Value["status"].(*types.AttributeValueMemberS); ok {
				li.Status = s.Value
			}
			o.Items = append(o.Items, li)
		}
	}
	return o, nil
}

func decodeOrderList(items []map[string]types.AttributeValue) ([]Order, error) {
	var out []Order
	for _, it := range items {
		o, err := decodeOrder(it)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

func encodeBill(b *Bill) (map[string]types.AttributeValue, error) {
	discounts := make([]types.AttributeValue, 0, len(b.Discounts))
	for _, d := range b.Discounts {
		discounts = append(discounts, &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id":          &types.AttributeValueMemberS{Value: d.ID},
			"type":        &types.AttributeValueMemberS{Value: d.Type},
			"amount":      &types.AttributeValueMemberN{Value: strconv.FormatInt(d.Amount, 10)},
			"description": &types.AttributeValueMemberS{Value: d.Description},
		}})
	}
	taxes := make([]types.AttributeValue, 0, len(b.Taxes))
	for _, t := range b.Taxes {
		taxes = append(taxes, &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id":              &types.AttributeValueMemberS{Value: t.ID},
			"name":            &types.AttributeValueMemberS{Value: t.Name},
			"rate_in_bps":     &types.AttributeValueMemberN{Value: strconv.Itoa(t.RateInBps)},
			"amount_in_paise": &types.AttributeValueMemberN{Value: strconv.FormatInt(t.AmountInPaise, 10)},
		}})
	}
	tids := make([]types.AttributeValue, 0, len(b.TableIDs))
	for _, t := range b.TableIDs {
		tids = append(tids, &types.AttributeValueMemberS{Value: t})
	}
	m := map[string]types.AttributeValue{
		"id":                      &types.AttributeValueMemberS{Value: b.ID},
		"session_id":              &types.AttributeValueMemberS{Value: b.SessionID},
		"payment_method":          &types.AttributeValueMemberS{Value: b.PaymentMethod},
		"payment_status":          &types.AttributeValueMemberS{Value: b.PaymentStatus},
		"created_at":              &types.AttributeValueMemberN{Value: strconv.FormatInt(b.CreatedAt, 10)},
		"updated_at":              &types.AttributeValueMemberN{Value: strconv.FormatInt(b.UpdatedAt, 10)},
		"table_ids":               &types.AttributeValueMemberL{Value: tids},
		"total_tax_in_paise":      &types.AttributeValueMemberN{Value: strconv.FormatInt(b.TotalTaxInPaise, 10)},
		"total_discount_in_paise": &types.AttributeValueMemberN{Value: strconv.FormatInt(b.TotalDiscountInPaise, 10)},
		"total_amount_in_paise":   &types.AttributeValueMemberN{Value: strconv.FormatInt(b.TotalAmountInPaise, 10)},
		"discounts":               &types.AttributeValueMemberL{Value: discounts},
		"taxes":                   &types.AttributeValueMemberL{Value: taxes},
	}
	if b.CustomerID != "" {
		m["customer_id"] = &types.AttributeValueMemberS{Value: b.CustomerID}
	}
	if b.StaffID != "" {
		m["staff_id"] = &types.AttributeValueMemberS{Value: b.StaffID}
	}
	return m, nil
}

func decodeBill(item map[string]types.AttributeValue) (*Bill, error) {
	b := &Bill{}
	if v, ok := item["id"].(*types.AttributeValueMemberS); ok {
		b.ID = v.Value
	}
	if v, ok := item["session_id"].(*types.AttributeValueMemberS); ok {
		b.SessionID = v.Value
	}
	if v, ok := item["payment_method"].(*types.AttributeValueMemberS); ok {
		b.PaymentMethod = v.Value
	}
	if v, ok := item["payment_status"].(*types.AttributeValueMemberS); ok {
		b.PaymentStatus = v.Value
	}
	if v, ok := item["customer_id"].(*types.AttributeValueMemberS); ok {
		b.CustomerID = v.Value
	}
	if v, ok := item["staff_id"].(*types.AttributeValueMemberS); ok {
		b.StaffID = v.Value
	}
	b.CreatedAt, _ = numAttr(item, "created_at")
	b.UpdatedAt, _ = numAttr(item, "updated_at")
	b.TotalTaxInPaise, _ = numAttr(item, "total_tax_in_paise")
	b.TotalDiscountInPaise, _ = numAttr(item, "total_discount_in_paise")
	b.TotalAmountInPaise, _ = numAttr(item, "total_amount_in_paise")
	if l, ok := item["table_ids"].(*types.AttributeValueMemberL); ok {
		for _, e := range l.Value {
			if sv, ok := e.(*types.AttributeValueMemberS); ok {
				b.TableIDs = append(b.TableIDs, sv.Value)
			}
		}
	}
	if l, ok := item["discounts"].(*types.AttributeValueMemberL); ok {
		for _, e := range l.Value {
			dm, ok := e.(*types.AttributeValueMemberM)
			if !ok {
				continue
			}
			d := DiscountType{}
			if s, ok := dm.Value["id"].(*types.AttributeValueMemberS); ok {
				d.ID = s.Value
			}
			if s, ok := dm.Value["type"].(*types.AttributeValueMemberS); ok {
				d.Type = s.Value
			}
			d.Amount, _ = numAttr(dm.Value, "amount")
			if s, ok := dm.Value["description"].(*types.AttributeValueMemberS); ok {
				d.Description = s.Value
			}
			b.Discounts = append(b.Discounts, d)
		}
	}
	if l, ok := item["taxes"].(*types.AttributeValueMemberL); ok {
		for _, e := range l.Value {
			tm, ok := e.(*types.AttributeValueMemberM)
			if !ok {
				continue
			}
			t := TaxType{}
			if s, ok := tm.Value["id"].(*types.AttributeValueMemberS); ok {
				t.ID = s.Value
			}
			if s, ok := tm.Value["name"].(*types.AttributeValueMemberS); ok {
				t.Name = s.Value
			}
			t.AmountInPaise, _ = numAttr(tm.Value, "amount_in_paise")
			rbs, _ := strAttr(tm.Value, "rate_in_bps")
			t.RateInBps, _ = strconv.Atoi(rbs)
			b.Taxes = append(b.Taxes, t)
		}
	}
	return b, nil
}

func numAttr(item map[string]types.AttributeValue, key string) (int64, error) {
	a, ok := item[key].(*types.AttributeValueMemberN)
	if !ok || a == nil {
		return 0, nil
	}
	return strconv.ParseInt(a.Value, 10, 64)
}

func strAttr(item map[string]types.AttributeValue, key string) (string, error) {
	a, ok := item[key].(*types.AttributeValueMemberS)
	if !ok || a == nil {
		return "", nil
	}
	return a.Value, nil
}

func atoiAttr(item map[string]types.AttributeValue, key string) (int, error) {
	a, ok := item[key].(*types.AttributeValueMemberN)
	if !ok || a == nil {
		return 0, nil
	}
	return strconv.Atoi(a.Value)
}
