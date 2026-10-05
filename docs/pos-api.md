# POS API contract (Staff app ⇄ Go backend)

Base URL: `/api/v1`. All POS routes need `Authorization: Bearer <jwt>`.
Staff tokens are pinned to a store; managers/owners send `X-Store-ID`.
Errors: `{"error": {"code": "...", "message": "..."}}`.
Money is integer VND. Times are RFC3339 UTC.

## Auth

| Method | Path | Body → Response |
|---|---|---|
| POST | `/auth/login` | `{email, password}` → `LoginResponse` |
| POST | `/auth/pin-login` | `{storeId, pin}` → `LoginResponse` (401 `invalid_pin`) |
| GET | `/auth/me` | → `UserView` |
| POST | `/pos/me/avatar` | multipart `file` (JPEG/PNG/WebP/GIF ≤ 10 MB) → `UserView` with the new `avatarUrl` |
| PUT | `/admin/users/:id/avatar` | `{avatarUrl}` (null clears; store manager+) → `{id, avatarUrl}` |

```
LoginResponse { accessToken, tokenType: "Bearer", expiresAt, user: UserView }
UserView {
  id, brandId, storeId, role, level, email, fullName, avatarUrl,
  brandName, brandLogoUrl, storeName        // null when not in scope
}
```

Demo tenant: `staff@dailybean.local` / `DailyBean123`, staff PIN `1234`.

## Menu

`GET /pos/menu` → `StoreMenu` (unchanged):

```
StoreMenu { storeId, categories: [{id, name, sortOrder, isActive}], items: [StoreItem] }
StoreItem {
  id, categoryId, name, description, imageUrl, basePrice, price, available, sortOrder,
  options: [OptionGroup]
}
OptionGroup { code, name, type: "single"|"multi", required, choices: [{code, name, priceDelta}] }
```

`PATCH /pos/menu/items/:id/availability` `{available: bool}` → `{itemId, available}`
(store-level override, used by the "Hết món" screen; broadcasts `menu.item.availability`).

## Tables (floor plan)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/pos/tables` | → `FloorPlan` |
| POST | `/pos/tables` | `TableInput` → 201 `Table` (store manager +) |
| PUT | `/pos/tables/:id` | `TableInput` → `Table` (store manager +) |
| DELETE | `/pos/tables/:id` | → 204, soft delete (store manager +) |

```
TableInput { name: "Bàn 11", zone?: "Ngoài sân", seats?: 4, sortOrder?: 11, isActive?: true }
FloorPlan {
  tables: [Table], zones: ["Trong nhà", "Ngoài sân"],
  total, free, serving, paid,      // counters for the header
  openRevenue,                     // money sitting on unpaid tables
  takeawayOpen                     // held takeaway orders, no table
}
Table {
  id, name, zone, seats, isActive,
  status: "free" | "serving" | "paid",
  orderId, orderNumber, orderStatus, paymentStatus, total, itemCount, openedAt, minutes  // null when free
}
```
A table is `serving` while its order is unpaid, `paid` once it is paid but the
order is still being prepared or waiting for pickup, and `free` again when the
order reaches `completed` / `cancelled`. A table serves several rounds a day,
so a `paid` table accepts a new order; a second order on a table that still
owes money is refused with 409 `table_busy`. Staff free a table by moving its
order to `completed`. `POST /pos/orders` accepts `tableId`;
the table's own name overwrites `tableLabel` so tickets and the plan agree.
Every order change broadcasts `tables.changed` on the store room.

## Customers (loyalty)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/pos/customers/lookup?phone=0901234567` | → `Customer` or 404 `not_found` |
| POST | `/pos/customers` | `{phone, name}` → 201 `Customer` (409 `conflict` if phone exists) |

```
Customer { id, phone, name, points, tier: "member"|"silver"|"gold" }
```
Tier: gold ≥ 100 points, silver ≥ 50, else member. Points earned per paid order = floor(total / 10000).

## Promotions

`GET /pos/promotions/:code` → `Promotion` or 404 `not_found`.

```
Promotion { code, name, type: "percent"|"fixed", value, minSubtotal }
```
Demo: `SALE10` = 10% off.

## Orders

```
CreateOrderInput {
  orderType: "dine_in"|"takeaway",
  tableId?: uuid,              // preferred; its name becomes tableLabel
  tableLabel?: "Bàn 05",       // free text fallback
  customerId?: uuid,
  promotionCode?: "SALE10",
  note?: string,
  items: [{ itemId, quantity, choices: [{group, code}], note? }]   // min 1
}
```
Server re-prices every line from the store menu (base/override price + choice deltas),
validates required single-choice groups, rejects unavailable items. Errors: 422 `invalid_order`.

| Method | Path | Body → Response |
|---|---|---|
| POST | `/pos/orders` | `CreateOrderInput` → 201 `Order` (status `open`, paymentStatus `unpaid`) |
| PUT | `/pos/orders/:id` | `CreateOrderInput` → `Order` (replace lines; 409 `not_open` unless status `open`) |
| POST | `/pos/orders/:id/pay` | `{method: "cash"|"vietqr"|"momo"|"zalopay", cashReceived?: int}` → `Order` (paymentStatus `paid`, status `preparing`; 409 `already_paid`) |
| GET | `/pos/orders?status=pending,preparing&source=app&date=2026-10-04` | → `{items: [Order]}` (date defaults to today in store tz; status/source are comma lists) |
| GET | `/pos/orders/:id` | → `Order` |
| PATCH | `/pos/orders/:id/status` | `{status}` → `Order` (409 `invalid_transition`) |
| GET | `/pos/orders/summary?date=` | → `Summary` |

Status machine:
`open → preparing` (via pay) · `open → cancelled` · `pending → preparing | rejected` ·
`preparing → ready | completed` · `ready → completed`.
A cafe hands drinks over at the table, so `preparing → completed` is allowed
directly: that is the "trả bàn" action on the floor plan.

```
Order {
  id, number: "A-1042" | "P-0012",      // A = app, P = POS
  source: "pos"|"app",
  orderType: "dine_in"|"takeaway"|"pickup",
  tableLabel, status, paymentStatus: "unpaid"|"paid",
  paymentMethod: null|"cash"|"vietqr"|"momo"|"zalopay",
  cashReceived, changeDue,
  customer: Customer | null,
  promotionCode, subtotal, discount, total, pointsEarned, note,
  items: [{
    id, itemId, name, quantity, unitPrice, lineTotal, optionsText,   // "Size L · Ít đá"
    choices: [{group, groupName, code, name, priceDelta}], note
  }],
  createdAt, paidAt, updatedAt,
  createdBy: {id, fullName} | null
}

Summary {
  date, orders, revenue,
  byMethod: {cash, vietqr, momo, zalopay},
  bySource: {pos, app},
  pendingApp            // app orders awaiting confirmation
}
```

Status labels (vi): open=Đang mở, pending=Chờ xác nhận, preparing=Đang pha, ready=Sẵn sàng,
completed=Đã giao, rejected=Đã từ chối, cancelled=Đã huỷ.

## Customer app (public, no auth)

`POST /app/stores/:storeId/orders`
```
{ customerName, customerPhone, orderType: "pickup", items: [...same as POS...],
  paymentMethod?: "momo"|"zalopay"|"vietqr", paid?: bool, note? }
```
→ 201 `Order` (source `app`, status `pending`). Used by the customer app and for demos.

## Realtime

`GET /ws?token=<jwt>` (WebSocket). Events: `{type, room, at, data}`

* `order.created` `{order: Order}` — new app order for this store
* `order.updated` `{order: Order}` — status / payment changed
* `menu.item.availability` `{itemId, isAvailable}`
* `tables.changed` `{storeId}` — refetch `/pos/tables`

## VietQR

Store bank details come with the store: `GET /pos/store` → `{id, name, address, phone, bankBin, bankCode, bankAccount, bankHolder}`.
The POS renders `https://img.vietqr.io/image/{bankCode}-{bankAccount}-compact2.png?amount={total}&addInfo={number}&accountName={bankHolder}`.
Demo store: VCB · 0011002234 · CAFE NHA MINH.
