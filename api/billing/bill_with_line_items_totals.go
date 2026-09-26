package billing

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var (
	errEmptyLineItems              = errors.New("line_items required")
	errCustomerIDRequiredForPoints = errors.New("customer_id required when applying points discount")
	errStateKeyRequired            = errors.New("state_key required for create")
	errBillIDRequired              = errors.New("id required for update")
	ErrBillNotFound                = errors.New("bill not found")
	errInsufficientPoints          = errors.New("insufficient points balance")
	errPointsExclusive             = errors.New("points cannot be combined with other discounts")
)

func lineItemsSubtotal(items []LineItemV0) int64 {
	var total int64
	for _, item := range items {
		if item.Quantity <= 0 {
			continue
		}
		total += item.Price * int64(item.Quantity)
	}
	return total
}

func sumTaxes(taxes []TaxType) int64 {
	var total int64
	for _, tax := range taxes {
		total += tax.AmountInPaise
	}
	return total
}

func hasPointsDiscount(discounts []DiscountType) bool {
	for _, d := range discounts {
		if d.Type == DiscountTypePoints && d.Amount > 0 {
			return true
		}
	}
	return false
}

func hasNonPointsDiscount(discounts []DiscountType) bool {
	for _, d := range discounts {
		if d.Type != DiscountTypePoints && d.Amount > 0 {
			return true
		}
	}
	return false
}

func requestedRedeemPoints(discounts []DiscountType) int64 {
	var requested int64
	for _, d := range discounts {
		if d.Type != DiscountTypePoints || d.Amount <= 0 {
			continue
		}
		pts := int64(0)
		if PaisePerPoint > 0 {
			pts = d.Amount / PaisePerPoint
		}
		if pts > requested {
			requested = pts
		}
	}
	return requested
}

// PointsEarnedFromDiscountedSubtotal is floor((subtotal - discounts) / Rs 50).
func PointsEarnedFromDiscountedSubtotal(subtotalPaise, discountPaise int64) int64 {
	base := subtotalPaise - discountPaise
	if base <= 0 || PaisePerEarnedPoint <= 0 {
		return 0
	}
	return base / PaisePerEarnedPoint
}

func lineItemsSubtotalPaise(items []LineItemV0) int64 {
	return lineItemsSubtotal(items)
}

type computedTotals struct {
	Discounts            []DiscountType
	Taxes                []TaxType
	TotalDiscountInPaise int64
	TotalTaxInPaise      int64
	TotalAmountInPaise   int64
	PointsRedeemed       int64
}

// recomputeTaxesAfterDiscount sets rate-based tax amounts on (subtotal − discount)
// and ceil-rupee round-off so POS UI and DynamoDB stay aligned.
func recomputeTaxesAfterDiscount(taxes []TaxType, taxablePaise int64) []TaxType {
	if len(taxes) == 0 {
		return nil
	}
	if taxablePaise < 0 {
		taxablePaise = 0
	}
	out := make([]TaxType, 0, len(taxes)+1)
	var rateTaxTotal int64
	hasRoundOff := false
	for _, t := range taxes {
		id := strings.ToLower(strings.TrimSpace(t.ID))
		if id == "round_off" || (t.RateInBps == 0 && strings.Contains(strings.ToLower(t.Name), "round")) {
			hasRoundOff = true
			continue
		}
		next := t
		if t.RateInBps > 0 {
			next.AmountInPaise = int64(math.Round(float64(taxablePaise) * float64(t.RateInBps) / 10000.0))
		}
		rateTaxTotal += next.AmountInPaise
		out = append(out, next)
	}
	preRound := taxablePaise + rateTaxTotal
	payableRupees := int64(math.Ceil(float64(preRound) / 100.0))
	roundOff := payableRupees*100 - preRound
	if roundOff < 0 {
		roundOff = 0
	}
	if roundOff > 0 || hasRoundOff {
		out = append(out, TaxType{
			ID:            "round_off",
			Name:          "Round off",
			RateInBps:     0,
			AmountInPaise: roundOff,
		})
	}
	return out
}

func computeBillTotals(
	lineItems []LineItemV0,
	discounts []DiscountType,
	taxes []TaxType,
	customerID string,
	pointsBalance int64,
	freezePointsDiscount bool,
	existingDiscounts []DiscountType,
) (computedTotals, error) {
	if len(lineItems) == 0 {
		return computedTotals{}, errEmptyLineItems
	}
	subtotal := lineItemsSubtotal(lineItems)

	outDiscounts := make([]DiscountType, 0, len(discounts))
	var totalDiscount int64
	var pointsRedeemed int64

	if freezePointsDiscount {
		for _, d := range existingDiscounts {
			if d.Type == DiscountTypePoints {
				outDiscounts = append(outDiscounts, d)
				totalDiscount += d.Amount
				if PaisePerPoint > 0 {
					pointsRedeemed = d.Amount / PaisePerPoint
				}
			}
		}
		for _, d := range discounts {
			if d.Type == DiscountTypePoints {
				continue
			}
			outDiscounts = append(outDiscounts, d)
			totalDiscount += d.Amount
		}
	} else {
		var nonPointsDiscount int64
		for _, d := range discounts {
			if d.Type == DiscountTypePoints {
				continue
			}
			outDiscounts = append(outDiscounts, d)
			nonPointsDiscount += d.Amount
		}
		totalDiscount = nonPointsDiscount

		if hasPointsDiscount(discounts) {
			if hasNonPointsDiscount(discounts) {
				return computedTotals{}, errPointsExclusive
			}
			customerID = strings.TrimSpace(customerID)
			if customerID == "" {
				return computedTotals{}, errCustomerIDRequiredForPoints
			}
			requested := requestedRedeemPoints(discounts)
			remaining := subtotal - nonPointsDiscount
			maxByBill := int64(0)
			if PaisePerPoint > 0 && remaining > 0 {
				maxByBill = remaining / PaisePerPoint
			}
			pointsToRedeem := requested
			if pointsToRedeem > pointsBalance {
				pointsToRedeem = pointsBalance
			}
			if pointsToRedeem > maxByBill {
				pointsToRedeem = maxByBill
			}
			if pointsToRedeem < 0 {
				pointsToRedeem = 0
			}
			if pointsToRedeem > 0 {
				pointsDiscount := pointsToRedeem * PaisePerPoint
				outDiscounts = append(outDiscounts, DiscountType{
					ID:          "points",
					Type:        DiscountTypePoints,
					Amount:      pointsDiscount,
					Description: fmt.Sprintf("%s points redeemed", strconv.FormatInt(pointsToRedeem, 10)),
				})
				totalDiscount += pointsDiscount
				pointsRedeemed = pointsToRedeem
			}
		}
	}

	totalDiscount = capDiscount(totalDiscount, subtotal)
	taxable := subtotal - totalDiscount
	outTaxes := recomputeTaxesAfterDiscount(taxes, taxable)
	totalTax := sumTaxes(outTaxes)
	return computedTotals{
		Discounts:            outDiscounts,
		Taxes:                outTaxes,
		TotalDiscountInPaise: totalDiscount,
		TotalTaxInPaise:      totalTax,
		TotalAmountInPaise:   taxable + totalTax,
		PointsRedeemed:       pointsRedeemed,
	}, nil
}

func capDiscount(discount, subtotal int64) int64 {
	if discount > subtotal {
		return subtotal
	}
	if discount < 0 {
		return 0
	}
	return discount
}
