package order

import (
	"context"
	"github.com/google/uuid"

	"coffeesos/internal/db"
)

// Adjust rewrites the lines of an order that was rung up wrong (an extra
// coffee, the wrong size) and records what changed. The order keeps its
// number and payment; the difference tells staff how much to collect from or
// hand back to the guest.
func (s *Service) Adjust(ctx context.Context, brandID, storeID, userID, id uuid.UUID, in AdjustInput) (View, int64, error) {
	o, err := s.q.GetOrder(ctx, db.GetOrderParams{ID: id, StoreID: storeID})
	if err != nil {
		return View{}, 0, err
	}
	switch o.Status {
	case StatusCancelled, StatusRejected:
		return View{}, 0, &StateError{Code: "order_closed", Msg: "đơn đã huỷ, không sửa được"}
	}
	cust, err := s.customerFor(ctx, brandID, in.CustomerID)
	if err != nil {
		return View{}, 0, err
	}
	if cust == nil && o.CustomerID != nil {
		// Keep the guest attached unless the edit explicitly drops them.
		if c, err := s.q.GetCustomerByID(ctx, db.GetCustomerByIDParams{ID: *o.CustomerID, BrandID: brandID}); err == nil {
			cust = &c
		}
	}
	cart, err := s.price(ctx, brandID, storeID, in.PromotionCode, in.Items)
	if err != nil {
		return View{}, 0, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return View{}, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := s.q.WithTx(tx)

	if err := qtx.DeleteOrderItems(ctx, id); err != nil {
		return View{}, 0, err
	}
	var promoCode, custName, custPhone *string
	var custID *uuid.UUID
	if cart.Promo != nil {
		promoCode = &cart.Promo.Code
	}
	if cust != nil {
		custID, custName, custPhone = &cust.ID, &cust.Name, &cust.Phone
	}
	orderType, tableLabel, tableID := o.OrderType, o.TableLabel, o.TableID
	if in.OrderType != "" {
		orderType = in.OrderType
	}
	updated, err := qtx.ReplaceOrderHeader(ctx, db.ReplaceOrderHeaderParams{
		ID: id, StoreID: storeID, OrderType: orderType, TableLabel: tableLabel, TableID: tableID,
		CustomerID: custID, CustomerName: custName, CustomerPhone: custPhone, PromotionCode: promoCode,
		Subtotal: cart.Subtotal, Discount: cart.Discount, Total: cart.Total, PointsEarned: cart.Points,
		Note: o.Note,
	})
	if err != nil {
		return View{}, 0, err
	}
	items, err := insertLines(ctx, qtx, id, cart.Lines)
	if err != nil {
		return View{}, 0, err
	}
	difference := updated.Total - o.Total
	if _, err := qtx.CreateOrderAdjustment(ctx, db.CreateOrderAdjustmentParams{
		OrderID: id, Reason: in.Reason, OldTotal: o.Total, NewTotal: updated.Total,
		Difference: difference, CreatedBy: &userID,
	}); err != nil {
		return View{}, 0, err
	}
	// Loyalty points followed the old total; move them by the same delta.
	if o.PaymentStatus == PaymentPaid && updated.CustomerID != nil {
		if delta := updated.PointsEarned - o.PointsEarned; delta != 0 {
			if _, err := qtx.AddCustomerPoints(ctx, db.AddCustomerPointsParams{
				ID: *updated.CustomerID, BrandID: brandID, Points: delta,
			}); err != nil {
				return View{}, 0, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, 0, err
	}

	v := s.withCreator(ctx, toView(updated, items, cust, nil), updated.CreatedBy)
	v.Adjustments = s.adjustments(ctx, id)
	s.notifyTables(storeID)
	s.broadcast(storeID, EventOrderUpdated, v)
	return v, difference, nil
}

// adjustments reads an order's correction history; best effort, an empty list
// is better than failing the read.
func (s *Service) adjustments(ctx context.Context, orderID uuid.UUID) []AdjustmentView {
	rows, err := s.q.ListOrderAdjustments(ctx, orderID)
	if err != nil {
		return nil
	}
	out := make([]AdjustmentView, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdjustmentView{
			ID: r.ID, Reason: r.Reason, OldTotal: r.OldTotal, NewTotal: r.NewTotal,
			Difference: r.Difference, ByName: r.ByName, At: r.CreatedAt,
		})
	}
	return out
}

// History lists the bills of one day, newest first, optionally filtered by
// status and a free-text query on number / customer.
func (s *Service) History(ctx context.Context, storeID uuid.UUID, date string, statuses []string, q string) ([]View, error) {
	from, to, _, err := s.dayRange(ctx, storeID, date)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.SearchOrders(ctx, db.SearchOrdersParams{
		StoreID: storeID, CreatedAt: from, CreatedAt_2: to, Column4: nonNil(statuses), Column5: q,
	})
	if err != nil {
		return nil, err
	}
	return s.hydrateMany(ctx, rows)
}
