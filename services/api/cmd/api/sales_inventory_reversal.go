package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// reverseVoidedSaleInventory restores the unreturned portion of every
// recipe-consumption movement associated with a voided sale. The sale row is
// already locked by the caller; product locks use the shared stock convention.
func reverseVoidedSaleInventory(ctx context.Context, tx pgx.Tx, tc tenantContext, saleID string) error {
	rows, err := tx.Query(ctx, `SELECT product_id,sum(-quantity_delta)::text FROM inventory_movements
		WHERE business_id=$1 AND branch_id=$2 AND reference_id=$3 AND movement_type='sale_consumption'
		GROUP BY product_id ORDER BY product_id`, tc.BusinessID, tc.BranchID, saleID)
	if err != nil {
		return err
	}
	type item struct{ product, quantity string }
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.product, &v.quantity); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, v := range items {
		if err = lockInventoryBalance(ctx, tx, tc.BusinessID, tc.BranchID, v.product); err != nil {
			return err
		}
		var already string
		err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND reference_id IN (SELECT id FROM sale_refunds WHERE business_id=$1 AND sale_id=$3) AND product_id=$4 AND movement_type='sale_refund_reversal'`, tc.BusinessID, tc.BranchID, saleID, v.product).Scan(&already)
		if err != nil {
			return err
		}
		var remaining string
		err = tx.QueryRow(ctx, `SELECT greatest($1::numeric-$2::numeric,0)::text`, v.quantity, already).Scan(&remaining)
		if err != nil {
			return err
		}
		if remaining == "0" || remaining == "0.00000000" {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by) VALUES($1,$2,$3,$4::numeric,'sale_void_reversal',$5,$6)`, tc.BusinessID, tc.BranchID, v.product, remaining, saleID, tc.UserID); err != nil {
			return err
		}
	}
	return nil
}

// reverseRefundInventory reverses the incremental proportional stock quantity
// represented by a newly recorded refund. Cumulative rounding is applied once
// per product so multiple partial refunds cannot over-return stock.
func reverseRefundInventory(ctx context.Context, tx pgx.Tx, tc tenantContext, saleID, refundID string, refundedTotal, saleTotal int64) error {
	if saleTotal <= 0 || refundedTotal <= 0 || refundedTotal > saleTotal {
		return fmt.Errorf("invalid refund inventory ratio")
	}
	rows, err := tx.Query(ctx, `SELECT product_id,sum(-quantity_delta)::text FROM inventory_movements
		WHERE business_id=$1 AND branch_id=$2 AND reference_id=$3 AND movement_type='sale_consumption'
		GROUP BY product_id ORDER BY product_id`, tc.BusinessID, tc.BranchID, saleID)
	if err != nil {
		return err
	}
	type item struct{ product, quantity string }
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.product, &v.quantity); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, v := range items {
		if err = lockInventoryBalance(ctx, tx, tc.BusinessID, tc.BranchID, v.product); err != nil {
			return err
		}
		var delta string
		err = tx.QueryRow(ctx, `SELECT greatest(round($1::numeric*$2::numeric/$3::numeric,8)-COALESCE((SELECT sum(m.quantity_delta) FROM inventory_movements m JOIN sale_refunds sr ON sr.business_id=m.business_id AND sr.id=m.reference_id WHERE m.business_id=$4 AND m.branch_id=$5 AND sr.sale_id=$6 AND m.product_id=$7 AND m.movement_type='sale_refund_reversal'),0),0)::text`, v.quantity, refundedTotal, saleTotal, tc.BusinessID, tc.BranchID, saleID, v.product).Scan(&delta)
		if err != nil {
			return err
		}
		if delta == "0" || delta == "0.00000000" {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by) VALUES($1,$2,$3,$4::numeric,'sale_refund_reversal',$5,$6)`, tc.BusinessID, tc.BranchID, v.product, delta, refundID, tc.UserID); err != nil {
			return err
		}
	}
	return nil
}
