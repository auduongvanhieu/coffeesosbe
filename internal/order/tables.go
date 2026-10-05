package order

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"coffeesos/internal/db"
	"coffeesos/internal/realtime"
)

const EventTablesChanged = "tables.changed"

// Table states the POS floor plan paints.
const (
	TableFree    = "free"    // nothing open here
	TableServing = "serving" // order still unpaid
	TablePaid    = "paid"    // paid, drinks still being made or waiting pickup
)

// TableView is one card on the floor plan.
type TableView struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Zone     string    `json:"zone"`
	Seats    int16     `json:"seats"`
	Status   string    `json:"status"`
	IsActive bool      `json:"isActive"`

	// Set when status != free.
	OrderID       *uuid.UUID `json:"orderId"`
	OrderNumber   *string    `json:"orderNumber"`
	OrderStatus   *string    `json:"orderStatus"`
	PaymentStatus *string    `json:"paymentStatus"`
	Total         *int64     `json:"total"`
	ItemCount     *int64     `json:"itemCount"`
	OpenedAt      *time.Time `json:"openedAt"`
	Minutes       *int64     `json:"minutes"` // how long the table has been busy
}

// FloorPlan is the whole overview screen in one response.
type FloorPlan struct {
	Tables   []TableView `json:"tables"`
	Zones    []string    `json:"zones"`
	Total    int         `json:"total"`
	Free     int         `json:"free"`
	Serving  int         `json:"serving"`
	Paid     int         `json:"paid"`
	Revenue  int64       `json:"openRevenue"` // money sitting on unpaid tables
	Takeaway int64       `json:"takeawayOpen"`
}

type TableInput struct {
	Name      string `json:"name" binding:"required,max=32"`
	Zone      string `json:"zone" binding:"max=32"`
	Seats     int16  `json:"seats" binding:"min=1,max=50"`
	SortOrder int32  `json:"sortOrder"`
	IsActive  *bool  `json:"isActive"`
}

// Tables builds the floor plan: every active table plus the order currently
// on it, so staff can see at a glance which tables are occupied.
func (s *Service) Tables(ctx context.Context, storeID uuid.UUID) (FloorPlan, error) {
	rows, err := s.q.ListStoreTables(ctx, storeID)
	if err != nil {
		return FloorPlan{}, err
	}
	busy, err := s.q.ListActiveTableOrders(ctx, storeID)
	if err != nil {
		return FloorPlan{}, err
	}
	plan := buildFloorPlan(rows, busy, time.Now())

	// Takeaway / counter orders have no table but still owe money.
	now := time.Now()
	open, err := s.q.ListOrders(ctx, db.ListOrdersParams{
		StoreID: storeID, CreatedAt: now.AddDate(0, 0, -1), CreatedAt_2: now.AddDate(0, 0, 1),
		Column4: []string{StatusOpen}, Column5: []string{},
	})
	if err != nil {
		return FloorPlan{}, err
	}
	for _, o := range open {
		if o.TableID == nil {
			plan.Takeaway += o.Total
		}
	}
	return plan, nil
}

// buildFloorPlan joins tables with the orders sitting on them. Pure, so the
// status rules and counters are testable without a database.
func buildFloorPlan(tables []db.StoreTable, busy []db.ListActiveTableOrdersRow, now time.Time) FloorPlan {
	byTable := make(map[uuid.UUID]db.ListActiveTableOrdersRow, len(busy))
	for _, b := range busy {
		if b.TableID != nil {
			byTable[*b.TableID] = b
		}
	}
	plan := FloorPlan{Tables: make([]TableView, 0, len(tables)), Zones: []string{}}
	seenZone := map[string]bool{}
	for _, t := range tables {
		v := TableView{ID: t.ID, Name: t.Name, Zone: t.Zone, Seats: t.Seats, IsActive: t.IsActive, Status: TableFree}
		if o, ok := byTable[t.ID]; ok {
			v.Status = TableServing
			if o.PaymentStatus == PaymentPaid {
				v.Status = TablePaid
			}
			mins := int64(now.Sub(o.CreatedAt).Minutes())
			if mins < 0 {
				mins = 0
			}
			v.OrderID, v.OrderNumber, v.OrderStatus = &o.ID, &o.Number, &o.Status
			v.PaymentStatus, v.Total, v.ItemCount = &o.PaymentStatus, &o.Total, &o.ItemCount
			v.OpenedAt, v.Minutes = &o.CreatedAt, &mins
		}
		switch v.Status {
		case TableFree:
			plan.Free++
		case TableServing:
			plan.Serving++
			plan.Revenue += *v.Total
		case TablePaid:
			plan.Paid++
		}
		if t.Zone != "" && !seenZone[t.Zone] {
			seenZone[t.Zone] = true
			plan.Zones = append(plan.Zones, t.Zone)
		}
		plan.Tables = append(plan.Tables, v)
	}
	plan.Total = len(plan.Tables)
	return plan
}

func (s *Service) CreateTable(ctx context.Context, storeID uuid.UUID, in TableInput) (db.StoreTable, error) {
	if in.Seats == 0 {
		in.Seats = 2
	}
	return s.q.CreateStoreTable(ctx, db.CreateStoreTableParams{
		StoreID: storeID, Name: strings.TrimSpace(in.Name), Zone: strings.TrimSpace(in.Zone),
		Seats: in.Seats, SortOrder: in.SortOrder,
	})
}

func (s *Service) UpdateTable(ctx context.Context, storeID, id uuid.UUID, in TableInput) (db.StoreTable, error) {
	cur, err := s.q.GetStoreTable(ctx, db.GetStoreTableParams{ID: id, StoreID: storeID})
	if err != nil {
		return db.StoreTable{}, err
	}
	active := cur.IsActive
	if in.IsActive != nil {
		active = *in.IsActive
	}
	if in.Seats == 0 {
		in.Seats = cur.Seats
	}
	return s.q.UpdateStoreTable(ctx, db.UpdateStoreTableParams{
		ID: id, StoreID: storeID, Name: strings.TrimSpace(in.Name), Zone: strings.TrimSpace(in.Zone),
		Seats: in.Seats, SortOrder: in.SortOrder, IsActive: active,
	})
}

func (s *Service) DeleteTable(ctx context.Context, storeID, id uuid.UUID) error {
	return s.q.DeleteStoreTable(ctx, db.DeleteStoreTableParams{ID: id, StoreID: storeID})
}

// notifyTables tells every POS in the store that the floor plan moved.
func (s *Service) notifyTables(storeID uuid.UUID) {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(realtime.StoreRoom(storeID), EventTablesChanged, map[string]any{"storeId": storeID})
}
