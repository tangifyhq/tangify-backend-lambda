package weborders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type WebOrderRepository struct {
	db *dynamodb.Client
}

func NewWebOrderRepository(db *dynamodb.Client) *WebOrderRepository {
	return &WebOrderRepository{db: db}
}

func (r *WebOrderRepository) GetByRef(ctx context.Context, orderRef string) (*WebOrder, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("web order store is not configured")
	}
	ref := NormalizeOrderRef(orderRef)
	if ref == "" {
		return nil, fmt.Errorf("order_ref required")
	}
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNameWebOrders),
		Key: map[string]types.AttributeValue{
			"order_ref": &types.AttributeValueMemberS{Value: ref},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return decodeWebOrder(out.Item)
}

func (r *WebOrderRepository) PutIfAbsent(ctx context.Context, order *WebOrder) (created bool, err error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("web order store is not configured")
	}
	if order == nil || order.OrderRef == "" {
		return false, fmt.Errorf("order required")
	}
	item, err := encodeWebOrder(order)
	if err != nil {
		return false, err
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(TableNameWebOrders),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(order_ref)"),
	})
	if err != nil {
		var cond *types.ConditionalCheckFailedException
		if errors.As(err, &cond) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *WebOrderRepository) ListByUser(ctx context.Context, userID string, limit int32) ([]WebOrder, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("web order store is not configured")
	}
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return nil, fmt.Errorf("user_id required")
	}
	if limit <= 0 {
		limit = 50
	}
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameWebOrders),
		IndexName:              aws.String(GSIUserPlaced),
		KeyConditionExpression: aws.String("user_id = :uid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":uid": &types.AttributeValueMemberS{Value: uid},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(limit),
	})
	if err != nil {
		return nil, err
	}
	orders := make([]WebOrder, 0, len(out.Items))
	for _, item := range out.Items {
		o, err := decodeWebOrder(item)
		if err != nil || o == nil {
			continue
		}
		orders = append(orders, *o)
	}
	return orders, nil
}

func encodeWebOrder(o *WebOrder) (map[string]types.AttributeValue, error) {
	linesJSON, err := json.Marshal(o.Lines)
	if err != nil {
		return nil, err
	}
	item := map[string]types.AttributeValue{
		"order_ref":          &types.AttributeValueMemberS{Value: o.OrderRef},
		"id":                 &types.AttributeValueMemberS{Value: o.ID},
		"user_id":            &types.AttributeValueMemberS{Value: o.UserID},
		"payment_id":         &types.AttributeValueMemberS{Value: o.PaymentID},
		"razorpay_order_id":  &types.AttributeValueMemberS{Value: o.RazorpayOrderID},
		"placed_at":          &types.AttributeValueMemberN{Value: strconv.FormatInt(o.PlacedAtUnix, 10)},
		"placed_at_iso":      &types.AttributeValueMemberS{Value: o.PlacedAt},
		"fulfillment":        &types.AttributeValueMemberS{Value: o.Fulfillment},
		"items_total_paise":  &types.AttributeValueMemberN{Value: strconv.FormatInt(o.ItemsTotalPaise, 10)},
		"cgst_paise":         &types.AttributeValueMemberN{Value: strconv.FormatInt(o.CgstPaise, 10)},
		"sgst_paise":         &types.AttributeValueMemberN{Value: strconv.FormatInt(o.SgstPaise, 10)},
		"delivery_fee_paise": &types.AttributeValueMemberN{Value: strconv.FormatInt(o.DeliveryFeePaise, 10)},
		"round_off_paise":    &types.AttributeValueMemberN{Value: strconv.FormatInt(o.RoundOffPaise, 10)},
		"total_paise":        &types.AttributeValueMemberN{Value: strconv.FormatInt(o.TotalPaise, 10)},
		"lines_json":         &types.AttributeValueMemberS{Value: string(linesJSON)},
		"status":             &types.AttributeValueMemberS{Value: o.Status},
	}
	if o.AddressLabel != "" {
		item["address_label"] = &types.AttributeValueMemberS{Value: o.AddressLabel}
	}
	if o.AddressSummary != "" {
		item["address_summary"] = &types.AttributeValueMemberS{Value: o.AddressSummary}
	}
	if o.ReceiverName != "" {
		item["receiver_name"] = &types.AttributeValueMemberS{Value: o.ReceiverName}
	}
	if o.ReceiverPhone != "" {
		item["receiver_phone"] = &types.AttributeValueMemberS{Value: o.ReceiverPhone}
	}
	return item, nil
}

func decodeWebOrder(item map[string]types.AttributeValue) (*WebOrder, error) {
	o := &WebOrder{Status: "paid"}
	if v, ok := item["order_ref"].(*types.AttributeValueMemberS); ok {
		o.OrderRef = v.Value
	}
	if v, ok := item["id"].(*types.AttributeValueMemberS); ok {
		o.ID = v.Value
	}
	if o.ID == "" {
		o.ID = o.OrderRef
	}
	if v, ok := item["user_id"].(*types.AttributeValueMemberS); ok {
		o.UserID = v.Value
	}
	if v, ok := item["payment_id"].(*types.AttributeValueMemberS); ok {
		o.PaymentID = v.Value
	}
	if v, ok := item["razorpay_order_id"].(*types.AttributeValueMemberS); ok {
		o.RazorpayOrderID = v.Value
	}
	if v, ok := item["placed_at"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		o.PlacedAtUnix = n
	}
	if v, ok := item["placed_at_iso"].(*types.AttributeValueMemberS); ok {
		o.PlacedAt = v.Value
	}
	if o.PlacedAt == "" && o.PlacedAtUnix > 0 {
		o.PlacedAt = time.UnixMilli(o.PlacedAtUnix).UTC().Format(time.RFC3339)
	}
	if v, ok := item["fulfillment"].(*types.AttributeValueMemberS); ok {
		o.Fulfillment = v.Value
	}
	if v, ok := item["items_total_paise"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		o.ItemsTotalPaise = n
	}
	if v, ok := item["cgst_paise"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		o.CgstPaise = n
	}
	if v, ok := item["sgst_paise"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		o.SgstPaise = n
	}
	if v, ok := item["delivery_fee_paise"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		o.DeliveryFeePaise = n
	}
	if v, ok := item["round_off_paise"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		o.RoundOffPaise = n
	}
	if v, ok := item["total_paise"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		o.TotalPaise = n
	}
	if v, ok := item["lines_json"].(*types.AttributeValueMemberS); ok && v.Value != "" {
		_ = json.Unmarshal([]byte(v.Value), &o.Lines)
	}
	if o.Lines == nil {
		o.Lines = []WebOrderLine{}
	}
	if v, ok := item["address_label"].(*types.AttributeValueMemberS); ok {
		o.AddressLabel = v.Value
	}
	if v, ok := item["address_summary"].(*types.AttributeValueMemberS); ok {
		o.AddressSummary = v.Value
	}
	if v, ok := item["receiver_name"].(*types.AttributeValueMemberS); ok {
		o.ReceiverName = v.Value
	}
	if v, ok := item["receiver_phone"].(*types.AttributeValueMemberS); ok {
		o.ReceiverPhone = v.Value
	}
	if v, ok := item["status"].(*types.AttributeValueMemberS); ok {
		o.Status = v.Value
	}
	if o.OrderRef == "" {
		return nil, fmt.Errorf("invalid web order item")
	}
	return o, nil
}
