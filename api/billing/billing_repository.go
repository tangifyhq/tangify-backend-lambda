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
	dynamodbClient *dynamodb.Client
}

var billingRepository *Repository

func NewRepository(dynamodbClient *dynamodb.Client) *Repository {
	if billingRepository == nil {
		billingRepository = &Repository{dynamodbClient: dynamodbClient}
		return billingRepository
	}
	return billingRepository
}

func (r *Repository) InsertBill(ctx context.Context, bill *BillType) error {
	if bill == nil {
		return fmt.Errorf("bill is nil")
	}
	ordersAttr := make([]types.AttributeValue, 0, len(*bill.Orders))
	for _, order := range *bill.Orders {
		itemsAttr := make([]types.AttributeValue, 0, len(*order.Items))
		for _, item := range *order.Items {
			itemsAttr = append(itemsAttr, &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"name":     &types.AttributeValueMemberS{Value: item.Name},
				"quantity": &types.AttributeValueMemberN{Value: strconv.Itoa(item.Quantity)},
				"price":    &types.AttributeValueMemberN{Value: strconv.FormatInt(item.Price, 10)},
			}})
		}

		orderAttrValue := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id":           &types.AttributeValueMemberS{Value: *order.ID},
			"items":        &types.AttributeValueMemberL{Value: itemsAttr},
			"total_price":  &types.AttributeValueMemberN{Value: strconv.FormatInt(*order.TotalPrice, 10)},
			"status":       &types.AttributeValueMemberS{Value: *order.Status},
			"ordered_at":   &types.AttributeValueMemberN{Value: strconv.FormatInt(*order.OrderedAt, 10)},
			"ready_at":     &types.AttributeValueMemberN{Value: strconv.FormatInt(*order.ReadyAt, 10)},
			"completed_at": &types.AttributeValueMemberN{Value: strconv.FormatInt(*order.CompletedAt, 10)},
		}}
		ordersAttr = append(ordersAttr, orderAttrValue)
	}

	discountsAttr := make([]types.AttributeValue, 0, len(*bill.Discounts))
	for _, d := range *bill.Discounts {
		discountsAttr = append(discountsAttr, &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id":          &types.AttributeValueMemberS{Value: d.ID},
			"type":        &types.AttributeValueMemberS{Value: d.Type},
			"amount":      &types.AttributeValueMemberN{Value: strconv.FormatInt(d.Amount, 10)},
			"description": &types.AttributeValueMemberS{Value: d.Description},
		}})
	}

	taxesAttr := make([]types.AttributeValue, 0, len(*bill.Taxes))
	for _, t := range *bill.Taxes {
		taxesAttr = append(taxesAttr, &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id":              &types.AttributeValueMemberS{Value: t.ID},
			"name":            &types.AttributeValueMemberS{Value: t.Name},
			"rate_in_bps":     &types.AttributeValueMemberN{Value: strconv.Itoa(t.RateInBps)},
			"amount_in_paise": &types.AttributeValueMemberN{Value: strconv.FormatInt(t.AmountInPaise, 10)},
		}})
	}

	item := map[string]types.AttributeValue{
		"id":                      &types.AttributeValueMemberS{Value: *bill.ID},
		"orders":                  &types.AttributeValueMemberL{Value: ordersAttr},
		"payment_method":          &types.AttributeValueMemberS{Value: *bill.PaymentMethod},
		"payment_status":          &types.AttributeValueMemberS{Value: *bill.PaymentStatus},
		"created_at":              &types.AttributeValueMemberN{Value: strconv.FormatInt(*bill.CreatedAt, 10)},
		"updated_at":              &types.AttributeValueMemberN{Value: strconv.FormatInt(*bill.UpdatedAt, 10)},
		"discounts":               &types.AttributeValueMemberL{Value: discountsAttr},
		"taxes":                   &types.AttributeValueMemberL{Value: taxesAttr},
		"table_id":                &types.AttributeValueMemberS{Value: *bill.TableID},
		"customer_id":             &types.AttributeValueMemberS{Value: *bill.CustomerID},
		"staff_id":                &types.AttributeValueMemberS{Value: *bill.StaffID},
		"total_tax_in_paise":      &types.AttributeValueMemberN{Value: strconv.FormatInt(*bill.TotalTaxInPaise, 10)},
		"total_discount_in_paise": &types.AttributeValueMemberN{Value: strconv.FormatInt(*bill.TotalDiscountInPaise, 10)},
		"total_amount_in_paise":   &types.AttributeValueMemberN{Value: strconv.FormatInt(*bill.TotalAmountInPaise, 10)},
	}

	_, err := r.dynamodbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNameBills),
		Item:      item,
	})
	return err
}

func parseNumericAttribute(item map[string]types.AttributeValue, key string) (int64, error) {
	attr, exists := item[key]
	if !exists || attr == nil {
		return 0, nil
	}
	nAttr, ok := attr.(*types.AttributeValueMemberN)
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(nAttr.Value, 10, 64)
}

func parseStringAttribute(item map[string]types.AttributeValue, key string) (string, error) {
	attr, exists := item[key]
	if !exists || attr == nil {
		return "", nil
	}
	sAttr, ok := attr.(*types.AttributeValueMemberS)
	if !ok {
		return "", nil
	}
	return sAttr.Value, nil
}

func (r *Repository) GetBillByID(ctx context.Context, billID string) (*BillType, error) {
	result, err := r.dynamodbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNameBills),
		Key: map[string]types.AttributeValue{
			"id":             &types.AttributeValueMemberS{Value: billID},
			"payment_status": &types.AttributeValueMemberS{Value: PaymentStatusPending},
		},
	})
	if err != nil {
		return nil, err
	}

	if len(result.Item) == 0 {
		return nil, nil
	}

	createdAt, err := parseNumericAttribute(result.Item, "created_at")
	if err != nil {
		return nil, err
	}
	updatedAt, err := parseNumericAttribute(result.Item, "updated_at")
	if err != nil {
		return nil, err
	}
	totalTaxInPaise, err := parseNumericAttribute(result.Item, "total_tax_in_paise")
	if err != nil {
		return nil, err
	}
	totalDiscountInPaise, err := parseNumericAttribute(result.Item, "total_discount_in_paise")
	if err != nil {
		return nil, err
	}
	totalAmountInPaise, err := parseNumericAttribute(result.Item, "total_amount_in_paise")
	if err != nil {
		return nil, err
	}

	orders := []BillOrderType{}
	if ordersAttr, ok := result.Item["orders"].(*types.AttributeValueMemberL); ok && ordersAttr != nil {
		for _, o := range ordersAttr.Value {
			orderMap, ok := o.(*types.AttributeValueMemberM)
			if !ok {
				continue
			}
			om := orderMap.Value
			totalPrice, _ := parseNumericAttribute(om, "total_price")
			orderedAt, _ := parseNumericAttribute(om, "ordered_at")
			readyAt, _ := parseNumericAttribute(om, "ready_at")
			completedAt, _ := parseNumericAttribute(om, "completed_at")

			items := make([]OrderItemType, 0)
			if itemsAttr, ok := om["items"].(*types.AttributeValueMemberL); ok && itemsAttr != nil {
				for _, itemVal := range itemsAttr.Value {
					itemMap, ok := itemVal.(*types.AttributeValueMemberM)
					if !ok {
						continue
					}
					im := itemMap.Value
					price, _ := parseNumericAttribute(im, "price")
					quantityStr, _ := parseStringAttribute(im, "quantity")
					quantity, _ := strconv.Atoi(quantityStr)
					name, _ := parseStringAttribute(im, "name")
					items = append(items, OrderItemType{
						Name:     name,
						Quantity: quantity,
						Price:    price,
					})
				}
			}

			id, _ := parseStringAttribute(om, "id")
			status, _ := parseStringAttribute(om, "status")

			orders = append(orders, BillOrderType{
				ID:          &id,
				Items:       &items,
				TotalPrice:  &totalPrice,
				Status:      &status,
				OrderedAt:   &orderedAt,
				ReadyAt:     &readyAt,
				CompletedAt: &completedAt,
			})
		}
	}

	discounts := []DiscountType{}
	if discountAttr, ok := result.Item["discounts"].(*types.AttributeValueMemberL); ok && discountAttr != nil {
		for _, d := range discountAttr.Value {
			dmap, ok := d.(*types.AttributeValueMemberM)
			if !ok {
				continue
			}
			dm := dmap.Value
			amount, _ := parseNumericAttribute(dm, "amount")
			id, _ := parseStringAttribute(dm, "id")
			discountType, _ := parseStringAttribute(dm, "type")
			description, _ := parseStringAttribute(dm, "description")
			discounts = append(discounts, DiscountType{
				ID:          id,
				Type:        discountType,
				Amount:      amount,
				Description: description,
			})
		}
	}

	taxes := []TaxType{}
	if taxesAttr, ok := result.Item["taxes"].(*types.AttributeValueMemberL); ok && taxesAttr != nil {
		for _, t := range taxesAttr.Value {
			tmap, ok := t.(*types.AttributeValueMemberM)
			if !ok {
				continue
			}
			tm := tmap.Value
			amountInPaise, _ := parseNumericAttribute(tm, "amount_in_paise")
			rateInBpsStr, _ := parseStringAttribute(tm, "rate_in_bps")
			rateInBps, _ := strconv.Atoi(rateInBpsStr)
			id, _ := parseStringAttribute(tm, "id")
			name, _ := parseStringAttribute(tm, "name")
			taxes = append(taxes, TaxType{
				ID:            id,
				Name:          name,
				RateInBps:     rateInBps,
				AmountInPaise: amountInPaise,
			})
		}
	}

	id, _ := parseStringAttribute(result.Item, "id")
	paymentMethod, _ := parseStringAttribute(result.Item, "payment_method")
	paymentStatus, _ := parseStringAttribute(result.Item, "payment_status")
	tableID, _ := parseStringAttribute(result.Item, "table_id")
	customerID, _ := parseStringAttribute(result.Item, "customer_id")
	staffID, _ := parseStringAttribute(result.Item, "staff_id")

	bill := BillType{
		ID:                   &id,
		Orders:               &orders,
		PaymentMethod:        &paymentMethod,
		PaymentStatus:        &paymentStatus,
		CreatedAt:            &createdAt,
		UpdatedAt:            &updatedAt,
		Discounts:            &discounts,
		Taxes:                &taxes,
		TableID:              &tableID,
		CustomerID:           &customerID,
		StaffID:              &staffID,
		TotalTaxInPaise:      &totalTaxInPaise,
		TotalDiscountInPaise: &totalDiscountInPaise,
		TotalAmountInPaise:   &totalAmountInPaise,
	}
	return &bill, nil
}

func (r *Repository) FetchLiveOrders() ([]OrderType, error) {
	return []OrderType{}, nil
}
