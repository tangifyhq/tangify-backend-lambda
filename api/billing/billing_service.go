package billing

import (
	"context"
	"fmt"
)

// ClockID provides ID generation and timestamps for bill/order creation.
type ClockID interface {
	GenerateUniqueID(prefix *string) string
	GetCurrentTimestamp() int64
}

type Service struct {
	repo *Repository
}

var billingService *Service

func NewService(repo *Repository) *Service {
	if billingService == nil {
		billingService = &Service{repo: repo}
		return billingService
	}
	return billingService
}

func (b *Service) ConstructBillID(tableID string, customerID string) string {
	return fmt.Sprintf("bill_%s:%s", tableID, customerID)
}

func (b *Service) GetBillByID(ctx context.Context, billID string) (*BillType, error) {
	return b.repo.GetBillByID(ctx, billID)
}

func (b *Service) AddOrderToBill(ctx context.Context, billID *string, order *OrderType, clock ClockID) (*BillType, error) {
	if order.OrderedAt == nil || *order.OrderedAt == 0 {
		return nil, fmt.Errorf("ordered_at is required")
	}
	fmt.Println("orderedAt: ", *order.OrderedAt)
	var billIDStr string
	if billID == nil {
		billIDStr = b.ConstructBillID(*order.TableID, *order.CustomerID)
	} else {
		billIDStr = *billID
	}

	bill, err := b.GetBillByID(ctx, billIDStr)
	if err != nil {
		fmt.Println("error getting bill: ", err)
		return nil, err
	}

	if bill == nil {
		fmt.Println("bill not found, will create new bill")
		prefixOrder := PrefixOrder
		id := clock.GenerateUniqueID(&prefixOrder)
		order.ID = &id
		if order.OrderedAt == nil || *order.OrderedAt == 0 {
			now := clock.GetCurrentTimestamp()
			order.OrderedAt = &now
		}

		billOrder := &BillOrderType{
			ID:          order.ID,
			Items:       order.Items,
			TotalPrice:  order.TotalPrice,
			Status:      order.Status,
			OrderedAt:   order.OrderedAt,
			ReadyAt:     order.ReadyAt,
			CompletedAt: order.CompletedAt,
		}

		createdAt := clock.GetCurrentTimestamp()
		updatedAt := clock.GetCurrentTimestamp()
		totalTaxInPaise := int64(0)
		totalDiscountInPaise := int64(0)
		totalAmountInPaise := int64(0)

		paymentMethod := PaymentMethodCash
		paymentStatus := PaymentStatusPending
		bill := BillType{
			ID:                   &billIDStr,
			Orders:               &[]BillOrderType{*billOrder},
			TableID:              order.TableID,
			CustomerID:           order.CustomerID,
			StaffID:              order.StaffID,
			CreatedAt:            &createdAt,
			UpdatedAt:            &updatedAt,
			TotalTaxInPaise:      &totalTaxInPaise,
			TotalDiscountInPaise: &totalDiscountInPaise,
			TotalAmountInPaise:   &totalAmountInPaise,
			Discounts:            &[]DiscountType{},
			Taxes:                &[]TaxType{},
			PaymentMethod:        &paymentMethod,
			PaymentStatus:        &paymentStatus,
		}
		fmt.Println("will insert bill: ", bill)
		err := b.repo.InsertBill(ctx, &bill)
		if err != nil {
			fmt.Println("error inserting bill: ", err)
			return nil, err
		}
		fmt.Println("bill inserted: ", bill)
		return &bill, nil
	}

	for _, existingOrder := range *bill.Orders {
		if *bill.StaffID == *order.StaffID && *existingOrder.OrderedAt == *order.OrderedAt {
			fmt.Println("order already exists, no changes needed")
			return bill, nil
		}
	}

	prefixOrder := PrefixOrder
	id := clock.GenerateUniqueID(&prefixOrder)
	order.ID = &id

	billOrder := BillOrderType{
		ID:          order.ID,
		Items:       order.Items,
		TotalPrice:  order.TotalPrice,
		Status:      order.Status,
		OrderedAt:   order.OrderedAt,
		ReadyAt:     order.ReadyAt,
		CompletedAt: order.CompletedAt,
	}

	orders := *bill.Orders
	orders = append(orders, billOrder)
	bill.Orders = &orders

	updatedAt := clock.GetCurrentTimestamp()
	bill.UpdatedAt = &updatedAt

	err = b.repo.InsertBill(ctx, bill)
	if err != nil {
		fmt.Println("error updating bill: ", err)
		return nil, err
	}
	fmt.Println("added order to billID: ", billIDStr)

	return bill, nil
}
