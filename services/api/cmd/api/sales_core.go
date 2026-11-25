package main

import (
	"errors"
	"math"
)

// saleQuoteLine is the server-priced representation of one discrete sellable unit.
// Fractional sale quantities are intentionally not accepted until the product/UOM
// rounding policy is explicitly decided.
type saleQuoteLine struct {
	ProductID string `json:"product_id"`
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Quantity  int64  `json:"quantity"`
	UnitPrice int64  `json:"unit_price_minor"`
	LineTotal int64  `json:"line_total_minor"`
}

type saleQuoteTotals struct {
	Subtotal int64 `json:"subtotal_minor"`
	Discount int64 `json:"discount_minor"`
	Tax      int64 `json:"tax_minor"`
	Service  int64 `json:"service_charge_minor"`
	Total    int64 `json:"total_minor"`
}

var errInvalidSaleQuantity = errors.New("sale quantity must be a positive whole number")
var errSaleAmountOverflow = errors.New("sale amount exceeds supported integer range")

func calculateSaleLine(productID, sku, name string, quantity, unitPriceMinor int64) (saleQuoteLine, error) {
	if quantity <= 0 {
		return saleQuoteLine{}, errInvalidSaleQuantity
	}
	if unitPriceMinor < 0 {
		return saleQuoteLine{}, errors.New("unit price cannot be negative")
	}
	if unitPriceMinor != 0 && quantity > math.MaxInt64/unitPriceMinor {
		return saleQuoteLine{}, errSaleAmountOverflow
	}
	return saleQuoteLine{ProductID: productID, SKU: sku, Name: name, Quantity: quantity, UnitPrice: unitPriceMinor, LineTotal: quantity * unitPriceMinor}, nil
}

func calculateSaleTotals(lines []saleQuoteLine) (saleQuoteTotals, error) {
	var out saleQuoteTotals
	if len(lines) == 0 {
		return out, errors.New("sale requires at least one line")
	}
	for _, line := range lines {
		if line.LineTotal < 0 || out.Subtotal > math.MaxInt64-line.LineTotal {
			return saleQuoteTotals{}, errSaleAmountOverflow
		}
		out.Subtotal += line.LineTotal
	}
	// Tax, service charge and discount are intentionally zero until the business
	// policy is approved. The quote makes this explicit rather than guessing.
	out.Total = out.Subtotal
	return out, nil
}
