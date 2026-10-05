// Package order implements the POS order flow: pricing a cart against the
// store menu, holding / paying orders, the app-order status machine, loyalty
// customers and promotion codes. Shapes here are documented in docs/pos-api.md.
package order

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"coffeesos/internal/db"
)

// Order sources, types, statuses and payment methods (mirrors the CHECKs in
// migration 000003).
const (
	SourcePOS = "pos"
	SourceApp = "app"

	TypeDineIn   = "dine_in"
	TypeTakeaway = "takeaway"
	TypePickup   = "pickup"

	StatusOpen      = "open"
	StatusPending   = "pending"
	StatusPreparing = "preparing"
	StatusReady     = "ready"
	StatusCompleted = "completed"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"

	PaymentUnpaid = "unpaid"
	PaymentPaid   = "paid"

	MethodCash    = "cash"
	MethodVietQR  = "vietqr"
	MethodMoMo    = "momo"
	MethodZaloPay = "zalopay"
)

// PointsPerVND: one loyalty point per 10.000đ paid.
const PointsPerVND = 10_000

// --- inputs ---

type ChoiceInput struct {
	Group string `json:"group" binding:"required,max=32"`
	Code  string `json:"code" binding:"required,max=32"`
}

type LineInput struct {
	ItemID   uuid.UUID     `json:"itemId" binding:"required"`
	Quantity int32         `json:"quantity" binding:"required,min=1,max=99"`
	Choices  []ChoiceInput `json:"choices" binding:"dive"`
	Note     *string       `json:"note" binding:"omitempty,max=200"`
}

type CreateInput struct {
	OrderType     string      `json:"orderType" binding:"required,oneof=dine_in takeaway"`
	TableLabel    *string     `json:"tableLabel" binding:"omitempty,max=32"`
	TableID       *uuid.UUID  `json:"tableId"`
	CustomerID    *uuid.UUID  `json:"customerId"`
	PromotionCode *string     `json:"promotionCode" binding:"omitempty,max=32"`
	Note          *string     `json:"note" binding:"omitempty,max=500"`
	Items         []LineInput `json:"items" binding:"required,min=1,max=100,dive"`
}

// AdjustInput corrects an order that was rung up wrong. The lines are
// replaced wholesale, exactly like CreateInput, plus a reason for the log.
type AdjustInput struct {
	CreateInput
	Reason string `json:"reason" binding:"required,min=3,max=200"`
}

type PayInput struct {
	Method       string `json:"method" binding:"required,oneof=cash vietqr momo zalopay"`
	CashReceived *int64 `json:"cashReceived" binding:"omitempty,min=0"`
}

type StatusInput struct {
	Status string `json:"status" binding:"required,oneof=open pending preparing ready completed rejected cancelled"`
}

type AppCreateInput struct {
	CustomerName  string      `json:"customerName" binding:"required,max=100"`
	CustomerPhone string      `json:"customerPhone" binding:"required,min=8,max=15"`
	OrderType     string      `json:"orderType" binding:"omitempty,oneof=pickup takeaway dine_in"`
	PaymentMethod *string     `json:"paymentMethod" binding:"omitempty,oneof=momo zalopay vietqr"`
	Paid          bool        `json:"paid"`
	Note          *string     `json:"note" binding:"omitempty,max=500"`
	Items         []LineInput `json:"items" binding:"required,min=1,max=100,dive"`
}

type CustomerInput struct {
	Phone string `json:"phone" binding:"required,min=8,max=15"`
	Name  string `json:"name" binding:"max=100"`
}

// --- views ---

type ChoiceView struct {
	Group      string `json:"group"`
	GroupName  string `json:"groupName"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	PriceDelta int64  `json:"priceDelta"`
}

type LineView struct {
	ID          uuid.UUID    `json:"id"`
	ItemID      *uuid.UUID   `json:"itemId"`
	Name        string       `json:"name"`
	Quantity    int32        `json:"quantity"`
	UnitPrice   int64        `json:"unitPrice"`
	LineTotal   int64        `json:"lineTotal"`
	OptionsText string       `json:"optionsText"`
	Choices     []ChoiceView `json:"choices"`
	Note        *string      `json:"note"`
}

type CustomerView struct {
	ID     uuid.UUID `json:"id"`
	Phone  string    `json:"phone"`
	Name   string    `json:"name"`
	Points int32     `json:"points"`
	Tier   string    `json:"tier"`
}

type UserRef struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"fullName"`
}

type View struct {
	ID            uuid.UUID     `json:"id"`
	Number        string        `json:"number"`
	Source        string        `json:"source"`
	OrderType     string        `json:"orderType"`
	TableLabel    *string       `json:"tableLabel"`
	TableID       *uuid.UUID    `json:"tableId"`
	Status        string        `json:"status"`
	PaymentStatus string        `json:"paymentStatus"`
	PaymentMethod *string       `json:"paymentMethod"`
	CashReceived  *int64        `json:"cashReceived"`
	ChangeDue     *int64        `json:"changeDue"`
	Customer      *CustomerView `json:"customer"`
	PromotionCode *string       `json:"promotionCode"`
	Subtotal      int64         `json:"subtotal"`
	Discount      int64         `json:"discount"`
	Total         int64         `json:"total"`
	PointsEarned  int32         `json:"pointsEarned"`
	Note          *string       `json:"note"`
	Items         []LineView    `json:"items"`
	CreatedAt     time.Time     `json:"createdAt"`
	PaidAt        *time.Time    `json:"paidAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	CreatedBy     *UserRef      `json:"createdBy"`

	// How many times this bill was corrected; the list screen shows a mark.
	AdjustmentCount int `json:"adjustmentCount"`

	// Filled on the single-order read; empty on list responses.
	Adjustments []AdjustmentView `json:"adjustments,omitempty"`
}

type PromotionView struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Value       int64  `json:"value"`
	MinSubtotal int64  `json:"minSubtotal"`
}

// AdjustmentView is one correction in an order's history.
type AdjustmentView struct {
	ID         uuid.UUID `json:"id"`
	Reason     string    `json:"reason"`
	OldTotal   int64     `json:"oldTotal"`
	NewTotal   int64     `json:"newTotal"`
	Difference int64     `json:"difference"` // > 0 collect more, < 0 give back
	ByName     *string   `json:"byName"`
	At         time.Time `json:"at"`
}

type Summary struct {
	Date       string           `json:"date"`
	Orders     int64            `json:"orders"`
	Revenue    int64            `json:"revenue"`
	ByMethod   map[string]int64 `json:"byMethod"`
	BySource   map[string]int64 `json:"bySource"`
	PendingApp int64            `json:"pendingApp"`
}

type StoreView struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Address     *string   `json:"address"`
	Phone       *string   `json:"phone"`
	Timezone    string    `json:"timezone"`
	BankBin     *string   `json:"bankBin"`
	BankCode    *string   `json:"bankCode"`
	BankAccount *string   `json:"bankAccount"`
	BankHolder  *string   `json:"bankHolder"`
}

// TierFor maps loyalty points to a tier label used by the clients.
func TierFor(points int32) string {
	switch {
	case points >= 100:
		return "gold"
	case points >= 50:
		return "silver"
	default:
		return "member"
	}
}

func toCustomerView(c db.Customer) CustomerView {
	return CustomerView{ID: c.ID, Phone: c.Phone, Name: c.Name, Points: c.Points, Tier: TierFor(c.Points)}
}

func toPromotionView(p db.Promotion) PromotionView {
	return PromotionView{Code: p.Code, Name: p.Name, Type: p.Type, Value: p.Value, MinSubtotal: p.MinSubtotal}
}

func toStoreView(s db.Store) StoreView {
	return StoreView{
		ID: s.ID, Name: s.Name, Address: s.Address, Phone: s.Phone, Timezone: s.Timezone,
		BankBin: s.BankBin, BankCode: s.BankCode, BankAccount: s.BankAccount, BankHolder: s.BankHolder,
	}
}

func toLineView(r db.OrderItem) LineView {
	var choices []ChoiceView
	if len(r.Choices) > 0 {
		_ = json.Unmarshal(r.Choices, &choices)
	}
	if choices == nil {
		choices = []ChoiceView{}
	}
	return LineView{
		ID: r.ID, ItemID: r.ItemID, Name: r.Name, Quantity: r.Quantity, UnitPrice: r.UnitPrice,
		LineTotal: r.LineTotal, OptionsText: r.OptionsText, Choices: choices, Note: r.Note,
	}
}

func toView(o db.Order, items []db.OrderItem, customer *db.Customer, createdBy *UserRef) View {
	lines := make([]LineView, 0, len(items))
	for _, it := range items {
		lines = append(lines, toLineView(it))
	}
	v := View{
		ID: o.ID, Number: o.Number, Source: o.Source, OrderType: o.OrderType, TableLabel: o.TableLabel,
		TableID: o.TableID, Status: o.Status, PaymentStatus: o.PaymentStatus, PaymentMethod: o.PaymentMethod,
		CashReceived: o.CashReceived, ChangeDue: o.ChangeDue, PromotionCode: o.PromotionCode,
		Subtotal: o.Subtotal, Discount: o.Discount, Total: o.Total, PointsEarned: o.PointsEarned,
		Note: o.Note, Items: lines, CreatedAt: o.CreatedAt, PaidAt: o.PaidAt, UpdatedAt: o.UpdatedAt,
		CreatedBy: createdBy,
	}
	if customer != nil {
		cv := toCustomerView(*customer)
		v.Customer = &cv
	} else if o.CustomerPhone != nil {
		// App orders from guests carry name/phone without a customer row.
		name := ""
		if o.CustomerName != nil {
			name = *o.CustomerName
		}
		v.Customer = &CustomerView{Phone: *o.CustomerPhone, Name: name, Tier: "member"}
	}
	return v
}
