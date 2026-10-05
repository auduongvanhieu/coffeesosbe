package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"coffeesos/internal/db"
	"coffeesos/internal/realtime"
)

const (
	EventOrderCreated = "order.created"
	EventOrderUpdated = "order.updated"
)

// StateError is a conflict with the order's current state (already paid,
// illegal transition, editing a non-open order). Handlers answer 409.
type StateError struct {
	Code string
	Msg  string
}

func (e *StateError) Error() string { return e.Msg }

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	hub  *realtime.Hub
}

func NewService(pool *pgxpool.Pool, q *db.Queries, hub *realtime.Hub) *Service {
	return &Service{pool: pool, q: q, hub: hub}
}

// --- lookups ---

func (s *Service) Store(ctx context.Context, storeID uuid.UUID) (StoreView, error) {
	st, err := s.q.GetStoreByID(ctx, storeID)
	if err != nil {
		return StoreView{}, err
	}
	return toStoreView(st), nil
}

func (s *Service) LookupCustomer(ctx context.Context, brandID uuid.UUID, phone string) (CustomerView, error) {
	c, err := s.q.GetCustomerByPhone(ctx, db.GetCustomerByPhoneParams{BrandID: brandID, Phone: normalizePhone(phone)})
	if err != nil {
		return CustomerView{}, err
	}
	return toCustomerView(c), nil
}

func (s *Service) CreateCustomer(ctx context.Context, brandID uuid.UUID, in CustomerInput) (CustomerView, error) {
	c, err := s.q.CreateCustomer(ctx, db.CreateCustomerParams{
		BrandID: brandID, Phone: normalizePhone(in.Phone), Name: strings.TrimSpace(in.Name),
	})
	if err != nil {
		return CustomerView{}, err
	}
	return toCustomerView(c), nil
}

func (s *Service) Promotion(ctx context.Context, brandID uuid.UUID, code string) (PromotionView, error) {
	p, err := s.q.GetPromotionByCode(ctx, db.GetPromotionByCodeParams{BrandID: brandID, Upper: strings.TrimSpace(code)})
	if err != nil {
		return PromotionView{}, err
	}
	return toPromotionView(p), nil
}

// --- create / replace ---

type createArgs struct {
	brandID, storeID uuid.UUID
	source, status   string
	orderType        string
	tableLabel       *string
	tableID          *uuid.UUID
	customer         *db.Customer
	guestName        *string
	guestPhone       *string
	promoCode        *string
	note             *string
	createdBy        *uuid.UUID
	paymentMethod    *string
	paid             bool
	items            []LineInput
}

// price resolves lines + promo for a store. Pure read.
func (s *Service) price(ctx context.Context, brandID, storeID uuid.UUID, promoCode *string, items []LineInput) (pricedCart, error) {
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ItemID)
	}
	rows, err := s.q.ListStoreItemIDs(ctx, db.ListStoreItemIDsParams{ID: storeID, Column2: ids})
	if err != nil {
		return pricedCart{}, err
	}
	lines, err := priceLines(rows, items)
	if err != nil {
		return pricedCart{}, err
	}
	var promo *db.Promotion
	if promoCode != nil && strings.TrimSpace(*promoCode) != "" {
		p, err := s.q.GetPromotionByCode(ctx, db.GetPromotionByCodeParams{BrandID: brandID, Upper: strings.TrimSpace(*promoCode)})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return pricedCart{}, invalid("mã giảm giá %s không hợp lệ", strings.ToUpper(strings.TrimSpace(*promoCode)))
			}
			return pricedCart{}, err
		}
		promo = &p
	}
	return totals(lines, promo)
}

func (s *Service) create(ctx context.Context, a createArgs) (View, error) {
	cart, err := s.price(ctx, a.brandID, a.storeID, a.promoCode, a.items)
	if err != nil {
		return View{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := s.q.WithTx(tx)

	no, err := qtx.NextOrderNo(ctx, a.storeID)
	if err != nil {
		return View{}, err
	}
	prefix := "P"
	if a.source == SourceApp {
		prefix = "A"
	}
	var (
		promoCode   *string
		custID      *uuid.UUID
		custName    = a.guestName
		custPhone   = a.guestPhone
		paidAt      *time.Time
		paymentStat = PaymentUnpaid
	)
	if cart.Promo != nil {
		promoCode = &cart.Promo.Code
	}
	if a.customer != nil {
		custID = &a.customer.ID
		custName = &a.customer.Name
		custPhone = &a.customer.Phone
	}
	if a.paid {
		now := time.Now()
		paidAt = &now
		paymentStat = PaymentPaid
	}
	o, err := qtx.CreateOrder(ctx, db.CreateOrderParams{
		BrandID: a.brandID, StoreID: a.storeID, OrderNo: no, Number: fmt.Sprintf("%s-%04d", prefix, no),
		Source: a.source, OrderType: a.orderType, TableLabel: a.tableLabel, TableID: a.tableID, Status: a.status,
		PaymentStatus: paymentStat, PaymentMethod: a.paymentMethod, PaidAt: paidAt,
		CustomerID: custID, CustomerName: custName, CustomerPhone: custPhone, PromotionCode: promoCode,
		Subtotal: cart.Subtotal, Discount: cart.Discount, Total: cart.Total, PointsEarned: cart.Points,
		Note: a.note, CreatedBy: a.createdBy,
	})
	if err != nil {
		return View{}, err
	}
	items, err := insertLines(ctx, qtx, o.ID, cart.Lines)
	if err != nil {
		return View{}, err
	}
	if a.paid && a.customer != nil && cart.Points > 0 {
		if _, err := qtx.AddCustomerPoints(ctx, db.AddCustomerPointsParams{ID: a.customer.ID, BrandID: a.brandID, Points: cart.Points}); err != nil {
			return View{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	v := toView(o, items, a.customer, nil)
	return v, nil
}

func insertLines(ctx context.Context, q *db.Queries, orderID uuid.UUID, lines []pricedLine) ([]db.OrderItem, error) {
	out := make([]db.OrderItem, 0, len(lines))
	for i, l := range lines {
		choices, _ := json.Marshal(l.Choices)
		itemID := l.ItemID
		row, err := q.CreateOrderItem(ctx, db.CreateOrderItemParams{
			OrderID: orderID, ItemID: &itemID, Name: l.Name, Quantity: l.Quantity, UnitPrice: l.UnitPrice,
			LineTotal: l.LineTotal, Choices: choices, OptionsText: l.OptionsText, Note: l.Note, SortOrder: int32(i),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// Create handles POST /pos/orders: a held (unpaid) POS order.
func (s *Service) Create(ctx context.Context, brandID, storeID, userID uuid.UUID, in CreateInput) (View, error) {
	cust, err := s.customerFor(ctx, brandID, in.CustomerID)
	if err != nil {
		return View{}, err
	}
	tableID, label, err := s.tableFor(ctx, storeID, in)
	if err != nil {
		return View{}, err
	}
	if err := s.assertTableFree(ctx, storeID, tableID); err != nil {
		return View{}, err
	}
	v, err := s.create(ctx, createArgs{
		brandID: brandID, storeID: storeID, source: SourcePOS, status: StatusOpen,
		orderType: in.OrderType, tableLabel: label, tableID: tableID, customer: cust, promoCode: in.PromotionCode,
		note: in.Note, createdBy: &userID, items: in.Items,
	})
	if err != nil {
		return View{}, err
	}
	s.notifyTables(storeID)
	s.broadcast(storeID, EventOrderCreated, v)
	return s.withCreator(ctx, v, &userID), nil
}

// tableFor resolves the table a dine-in order sits at. The table's own name
// wins over any label the client sent, so tickets and the floor plan agree.
func (s *Service) tableFor(ctx context.Context, storeID uuid.UUID, in CreateInput) (*uuid.UUID, *string, error) {
	if in.OrderType == TypeTakeaway || in.TableID == nil {
		if in.OrderType == TypeTakeaway {
			return nil, nil, nil
		}
		return nil, in.TableLabel, nil
	}
	t, err := s.q.GetStoreTable(ctx, db.GetStoreTableParams{ID: *in.TableID, StoreID: storeID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, invalid("bàn không thuộc cửa hàng này")
		}
		return nil, nil, err
	}
	return &t.ID, &t.Name, nil
}

// assertTableFree refuses a second unpaid order on the same table: a paid
// table may start a new round (guests leave, new guests sit down), but two
// open bills on one table would be a mistake, not a feature.
func (s *Service) assertTableFree(ctx context.Context, storeID uuid.UUID, tableID *uuid.UUID) error {
	if tableID == nil {
		return nil
	}
	o, err := s.q.GetUnpaidOrderByTable(ctx, db.GetUnpaidOrderByTableParams{TableID: tableID, StoreID: storeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return &StateError{Code: "table_busy", Msg: fmt.Sprintf("bàn đang có đơn %s chưa thanh toán", o.Number)}
}

// Replace handles PUT /pos/orders/:id: re-price and overwrite an open order.
func (s *Service) Replace(ctx context.Context, brandID, storeID, id uuid.UUID, in CreateInput) (View, error) {
	o, err := s.q.GetOrder(ctx, db.GetOrderParams{ID: id, StoreID: storeID})
	if err != nil {
		return View{}, err
	}
	if o.Status != StatusOpen {
		return View{}, &StateError{Code: "not_open", Msg: "chỉ sửa được đơn đang mở"}
	}
	cust, err := s.customerFor(ctx, brandID, in.CustomerID)
	if err != nil {
		return View{}, err
	}
	tableID, label, err := s.tableFor(ctx, storeID, in)
	if err != nil {
		return View{}, err
	}
	cart, err := s.price(ctx, brandID, storeID, in.PromotionCode, in.Items)
	if err != nil {
		return View{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := s.q.WithTx(tx)
	if err := qtx.DeleteOrderItems(ctx, id); err != nil {
		return View{}, err
	}
	var promoCode, custName, custPhone *string
	var custID *uuid.UUID
	if cart.Promo != nil {
		promoCode = &cart.Promo.Code
	}
	if cust != nil {
		custID, custName, custPhone = &cust.ID, &cust.Name, &cust.Phone
	}
	o, err = qtx.ReplaceOrderHeader(ctx, db.ReplaceOrderHeaderParams{
		ID: id, StoreID: storeID, OrderType: in.OrderType, TableLabel: label, TableID: tableID,
		CustomerID: custID, CustomerName: custName, CustomerPhone: custPhone, PromotionCode: promoCode,
		Subtotal: cart.Subtotal, Discount: cart.Discount, Total: cart.Total, PointsEarned: cart.Points, Note: in.Note,
	})
	if err != nil {
		return View{}, err
	}
	items, err := insertLines(ctx, qtx, id, cart.Lines)
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	v := s.withCreator(ctx, toView(o, items, cust, nil), o.CreatedBy)
	s.notifyTables(storeID)
	s.broadcast(storeID, EventOrderUpdated, v)
	return v, nil
}

// CreateFromApp handles the public customer-app endpoint.
func (s *Service) CreateFromApp(ctx context.Context, store db.Store, in AppCreateInput) (View, error) {
	phone := normalizePhone(in.CustomerPhone)
	cust, err := s.q.GetCustomerByPhone(ctx, db.GetCustomerByPhoneParams{BrandID: store.BrandID, Phone: phone})
	if errors.Is(err, pgx.ErrNoRows) {
		cust, err = s.q.CreateCustomer(ctx, db.CreateCustomerParams{BrandID: store.BrandID, Phone: phone, Name: strings.TrimSpace(in.CustomerName)})
	}
	if err != nil {
		return View{}, err
	}
	orderType := in.OrderType
	if orderType == "" {
		orderType = TypePickup
	}
	paid := in.Paid && in.PaymentMethod != nil
	v, err := s.create(ctx, createArgs{
		brandID: store.BrandID, storeID: store.ID, source: SourceApp, status: StatusPending,
		orderType: orderType, customer: &cust, note: in.Note, paymentMethod: in.PaymentMethod, paid: paid, items: in.Items,
	})
	if err != nil {
		return View{}, err
	}
	s.broadcast(store.ID, EventOrderCreated, v)
	return v, nil
}

// --- pay / status ---

func (s *Service) Pay(ctx context.Context, brandID, storeID, id uuid.UUID, in PayInput) (View, error) {
	o, err := s.q.GetOrder(ctx, db.GetOrderParams{ID: id, StoreID: storeID})
	if err != nil {
		return View{}, err
	}
	if o.PaymentStatus == PaymentPaid {
		return View{}, &StateError{Code: "already_paid", Msg: "đơn đã thanh toán"}
	}
	switch o.Status {
	case StatusCancelled, StatusRejected:
		return View{}, &StateError{Code: "invalid_transition", Msg: "đơn đã huỷ"}
	}
	var cashReceived, changeDue *int64
	if in.Method == MethodCash {
		if in.CashReceived == nil {
			in.CashReceived = &o.Total
		}
		if *in.CashReceived < o.Total {
			return View{}, invalid("khách đưa chưa đủ tiền")
		}
		change := *in.CashReceived - o.Total
		cashReceived, changeDue = in.CashReceived, &change
	}
	status := o.Status
	if status == StatusOpen {
		status = StatusPreparing
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := s.q.WithTx(tx)
	method := in.Method
	o, err = qtx.PayOrder(ctx, db.PayOrderParams{
		ID: id, StoreID: storeID, PaymentMethod: &method, CashReceived: cashReceived, ChangeDue: changeDue, Status: status,
	})
	if err != nil {
		return View{}, err
	}
	var cust *db.Customer
	if o.CustomerID != nil {
		c, err := qtx.AddCustomerPoints(ctx, db.AddCustomerPointsParams{ID: *o.CustomerID, BrandID: brandID, Points: o.PointsEarned})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return View{}, err
		}
		if err == nil {
			cust = &c
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	v, err := s.hydrate(ctx, o, cust)
	if err != nil {
		return View{}, err
	}
	s.broadcast(storeID, EventOrderUpdated, v)
	s.notifyTables(storeID)
	return v, nil
}

var transitions = map[string][]string{
	StatusOpen:      {StatusCancelled},
	StatusPending:   {StatusPreparing, StatusRejected},
	StatusPreparing: {StatusReady, StatusCompleted},
	StatusReady:     {StatusCompleted},
}

func (s *Service) SetStatus(ctx context.Context, storeID, id uuid.UUID, status string) (View, error) {
	o, err := s.q.GetOrder(ctx, db.GetOrderParams{ID: id, StoreID: storeID})
	if err != nil {
		return View{}, err
	}
	allowed := false
	for _, to := range transitions[o.Status] {
		if to == status {
			allowed = true
			break
		}
	}
	if !allowed {
		return View{}, &StateError{Code: "invalid_transition", Msg: fmt.Sprintf("không thể chuyển %s → %s", o.Status, status)}
	}
	o, err = s.q.SetOrderStatus(ctx, db.SetOrderStatusParams{ID: id, StoreID: storeID, Status: status})
	if err != nil {
		return View{}, err
	}
	v, err := s.hydrate(ctx, o, nil)
	if err != nil {
		return View{}, err
	}
	s.broadcast(storeID, EventOrderUpdated, v)
	s.notifyTables(storeID)
	return v, nil
}

// --- read ---

func (s *Service) Get(ctx context.Context, storeID, id uuid.UUID) (View, error) {
	o, err := s.q.GetOrder(ctx, db.GetOrderParams{ID: id, StoreID: storeID})
	if err != nil {
		return View{}, err
	}
	v, err := s.hydrate(ctx, o, nil)
	if err != nil {
		return View{}, err
	}
	v.Adjustments = s.adjustments(ctx, id)
	return v, nil
}

type ListFilter struct {
	Date     string // YYYY-MM-DD in store tz; empty = today
	Statuses []string
	Sources  []string
}

func (s *Service) List(ctx context.Context, storeID uuid.UUID, f ListFilter) ([]View, error) {
	from, to, _, err := s.dayRange(ctx, storeID, f.Date)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListOrders(ctx, db.ListOrdersParams{
		StoreID: storeID, CreatedAt: from, CreatedAt_2: to, Column4: nonNil(f.Statuses), Column5: nonNil(f.Sources),
	})
	if err != nil {
		return nil, err
	}
	return s.hydrateMany(ctx, rows)
}

func (s *Service) Summary(ctx context.Context, storeID uuid.UUID, date string) (Summary, error) {
	from, to, day, err := s.dayRange(ctx, storeID, date)
	if err != nil {
		return Summary{}, err
	}
	r, err := s.q.OrderSummary(ctx, db.OrderSummaryParams{StoreID: storeID, CreatedAt: from, CreatedAt_2: to})
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		Date: day, Orders: r.Orders, Revenue: r.Revenue,
		ByMethod:   map[string]int64{MethodCash: r.Cash, MethodVietQR: r.Vietqr, MethodMoMo: r.Momo, MethodZaloPay: r.Zalopay},
		BySource:   map[string]int64{SourcePOS: r.PosOrders, SourceApp: r.AppOrders},
		PendingApp: r.PendingApp,
	}, nil
}

// SetStoreAvailability toggles a store-level sold-out override.
func (s *Service) SetStoreAvailability(ctx context.Context, storeID, itemID uuid.UUID, available bool) error {
	rows, err := s.q.ListStoreItemIDs(ctx, db.ListStoreItemIDsParams{ID: storeID, Column2: []uuid.UUID{itemID}})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return pgx.ErrNoRows
	}
	if _, err := s.q.UpsertStoreItemAvailability(ctx, db.UpsertStoreItemAvailabilityParams{StoreID: storeID, ItemID: itemID, IsAvailable: available}); err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.Broadcast(realtime.StoreRoom(storeID), "menu.item.availability", map[string]any{
			"itemId": itemID, "isAvailable": available && rows[0].IsAvailable,
		})
	}
	return nil
}

// --- helpers ---

func (s *Service) customerFor(ctx context.Context, brandID uuid.UUID, id *uuid.UUID) (*db.Customer, error) {
	if id == nil {
		return nil, nil
	}
	c, err := s.q.GetCustomerByID(ctx, db.GetCustomerByIDParams{ID: *id, BrandID: brandID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invalid("khách hàng không tồn tại")
		}
		return nil, err
	}
	return &c, nil
}

func (s *Service) withCreator(ctx context.Context, v View, userID *uuid.UUID) View {
	if userID == nil {
		return v
	}
	if u, err := s.q.GetUserByID(ctx, *userID); err == nil {
		v.CreatedBy = &UserRef{ID: u.ID, FullName: u.FullName}
	}
	return v
}

func (s *Service) hydrate(ctx context.Context, o db.Order, cust *db.Customer) (View, error) {
	views, err := s.hydrateMany(ctx, []db.Order{o})
	if err != nil {
		return View{}, err
	}
	v := views[0]
	if cust != nil {
		cv := toCustomerView(*cust)
		v.Customer = &cv
	}
	return v, nil
}

func (s *Service) hydrateMany(ctx context.Context, orders []db.Order) ([]View, error) {
	if len(orders) == 0 {
		return []View{}, nil
	}
	ids := make([]uuid.UUID, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.ID)
	}
	items, err := s.q.ListOrderItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	byOrder := map[uuid.UUID][]db.OrderItem{}
	for _, it := range items {
		byOrder[it.OrderID] = append(byOrder[it.OrderID], it)
	}
	adjCount := map[uuid.UUID]int{}
	if rows, err := s.q.CountAdjustmentsByOrders(ctx, ids); err == nil {
		for _, r := range rows {
			adjCount[r.OrderID] = int(r.N)
		}
	}
	custCache := map[uuid.UUID]*db.Customer{}
	userCache := map[uuid.UUID]*UserRef{}
	out := make([]View, 0, len(orders))
	for _, o := range orders {
		var cust *db.Customer
		if o.CustomerID != nil {
			if c, ok := custCache[*o.CustomerID]; ok {
				cust = c
			} else if c, err := s.q.GetCustomerByID(ctx, db.GetCustomerByIDParams{ID: *o.CustomerID, BrandID: o.BrandID}); err == nil {
				cust = &c
				custCache[*o.CustomerID] = cust
			}
		}
		var by *UserRef
		if o.CreatedBy != nil {
			if u, ok := userCache[*o.CreatedBy]; ok {
				by = u
			} else if u, err := s.q.GetUserByID(ctx, *o.CreatedBy); err == nil {
				by = &UserRef{ID: u.ID, FullName: u.FullName}
				userCache[*o.CreatedBy] = by
			}
		}
		v := toView(o, byOrder[o.ID], cust, by)
		v.AdjustmentCount = adjCount[o.ID]
		out = append(out, v)
	}
	return out, nil
}

// dayRange returns [start, end) of a calendar day in the store's timezone.
func (s *Service) dayRange(ctx context.Context, storeID uuid.UUID, date string) (time.Time, time.Time, string, error) {
	st, err := s.q.GetStoreByID(ctx, storeID)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600)
	}
	var day time.Time
	if date == "" {
		now := time.Now().In(loc)
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	} else {
		day, err = time.ParseInLocation("2006-01-02", date, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", invalid("date phải có dạng YYYY-MM-DD")
		}
	}
	return day, day.AddDate(0, 0, 1), day.Format("2006-01-02"), nil
}

func (s *Service) broadcast(storeID uuid.UUID, event string, v View) {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(realtime.StoreRoom(storeID), event, map[string]any{"order": v})
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func normalizePhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if strings.HasPrefix(out, "84") && len(out) >= 11 {
		out = "0" + out[2:]
	}
	return out
}
