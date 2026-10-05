package order

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"coffeesos/internal/db"
)

func TestBuildFloorPlanMarksOccupiedTables(t *testing.T) {
	now := time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)
	t1, t2, t3 := uuid.New(), uuid.New(), uuid.New()
	tables := []db.StoreTable{
		{ID: t1, Name: "Bàn 01", Zone: "Trong nhà", Seats: 4, IsActive: true},
		{ID: t2, Name: "Bàn 02", Zone: "Trong nhà", Seats: 4, IsActive: true},
		{ID: t3, Name: "Bàn 07", Zone: "Ngoài sân", Seats: 2, IsActive: true},
	}
	busy := []db.ListActiveTableOrdersRow{
		{TableID: &t1, ID: uuid.New(), Number: "P-0001", Status: StatusOpen, PaymentStatus: PaymentUnpaid,
			Total: 68000, ItemCount: 2, CreatedAt: now.Add(-25 * time.Minute)},
		{TableID: &t3, ID: uuid.New(), Number: "P-0002", Status: StatusPreparing, PaymentStatus: PaymentPaid,
			Total: 45000, ItemCount: 1, CreatedAt: now.Add(-5 * time.Minute)},
	}

	plan := buildFloorPlan(tables, busy, now)

	if plan.Total != 3 || plan.Free != 1 || plan.Serving != 1 || plan.Paid != 1 {
		t.Errorf("counters = total %d free %d serving %d paid %d", plan.Total, plan.Free, plan.Serving, plan.Paid)
	}
	// Only unpaid tables owe money.
	if plan.Revenue != 68000 {
		t.Errorf("openRevenue = %d, want 68000", plan.Revenue)
	}
	if got := plan.Tables[0]; got.Status != TableServing || *got.Minutes != 25 || *got.ItemCount != 2 {
		t.Errorf("Bàn 01 = %+v", got)
	}
	if got := plan.Tables[1]; got.Status != TableFree || got.OrderID != nil {
		t.Errorf("Bàn 02 should be free, got %+v", got)
	}
	if got := plan.Tables[2]; got.Status != TablePaid {
		t.Errorf("Bàn 07 should be paid, got %+v", got)
	}
	if len(plan.Zones) != 2 || plan.Zones[0] != "Trong nhà" || plan.Zones[1] != "Ngoài sân" {
		t.Errorf("zones = %v", plan.Zones)
	}
}

func TestBuildFloorPlanClampsClockSkew(t *testing.T) {
	now := time.Now()
	id := uuid.New()
	plan := buildFloorPlan(
		[]db.StoreTable{{ID: id, Name: "Bàn 01", IsActive: true}},
		[]db.ListActiveTableOrdersRow{{TableID: &id, Number: "P-1", Status: StatusOpen,
			PaymentStatus: PaymentUnpaid, CreatedAt: now.Add(2 * time.Minute)}},
		now,
	)
	if *plan.Tables[0].Minutes != 0 {
		t.Errorf("minutes = %d, want 0 for a future timestamp", *plan.Tables[0].Minutes)
	}
}

func TestTransitionsLetAPreparingOrderCloseTheTable(t *testing.T) {
	// A cafe hands the drink over at the table, so staff must be able to free
	// the table straight from "preparing" without stepping through "ready".
	for _, want := range []string{StatusReady, StatusCompleted} {
		found := false
		for _, to := range transitions[StatusPreparing] {
			if to == want {
				found = true
			}
		}
		if !found {
			t.Errorf("preparing should allow %s", want)
		}
	}
	// Paid tables may start a new round; finished orders are final.
	if len(transitions[StatusCompleted]) != 0 {
		t.Errorf("completed should be terminal, got %v", transitions[StatusCompleted])
	}
}
