-- name: CreateOrder :one
INSERT INTO orders (
    brand_id, store_id, order_no, number, source, order_type, table_label, table_id, status,
    payment_status, payment_method, paid_at,
    customer_id, customer_name, customer_phone, promotion_code,
    subtotal, discount, total, points_earned, note, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12,
    $13, $14, $15, $16,
    $17, $18, $19, $20, $21, $22
)
RETURNING *;

-- name: ReplaceOrderHeader :one
UPDATE orders
SET order_type = $3, table_label = $4, table_id = $14, customer_id = $5, customer_name = $6, customer_phone = $7,
    promotion_code = $8, subtotal = $9, discount = $10, total = $11, points_earned = $12,
    note = $13, updated_at = now()
WHERE id = $1 AND store_id = $2
RETURNING *;

-- name: CreateOrderItem :one
INSERT INTO order_items (order_id, item_id, name, quantity, unit_price, line_total, choices, options_text, note, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: DeleteOrderItems :exec
DELETE FROM order_items WHERE order_id = $1;

-- name: ListOrderItems :many
SELECT * FROM order_items
WHERE order_id = ANY($1::uuid[])
ORDER BY order_id, sort_order;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1 AND store_id = $2;

-- name: ListOrders :many
SELECT * FROM orders
WHERE store_id = $1
  AND created_at >= $2 AND created_at < $3
  AND (cardinality($4::text[]) = 0 OR status = ANY($4::text[]))
  AND (cardinality($5::text[]) = 0 OR source = ANY($5::text[]))
ORDER BY created_at DESC;

-- name: PayOrder :one
UPDATE orders
SET payment_status = 'paid', payment_method = $3, cash_received = $4, change_due = $5,
    paid_at = now(), status = $6, updated_at = now()
WHERE id = $1 AND store_id = $2
RETURNING *;

-- name: SetOrderStatus :one
UPDATE orders SET status = $3, updated_at = now()
WHERE id = $1 AND store_id = $2
RETURNING *;

-- name: OrderSummary :one
SELECT
  count(*) FILTER (WHERE payment_status = 'paid')::bigint                                   AS orders,
  COALESCE(sum(total) FILTER (WHERE payment_status = 'paid'), 0)::bigint                    AS revenue,
  COALESCE(sum(total) FILTER (WHERE payment_status = 'paid' AND payment_method = 'cash'), 0)::bigint    AS cash,
  COALESCE(sum(total) FILTER (WHERE payment_status = 'paid' AND payment_method = 'vietqr'), 0)::bigint  AS vietqr,
  COALESCE(sum(total) FILTER (WHERE payment_status = 'paid' AND payment_method = 'momo'), 0)::bigint    AS momo,
  COALESCE(sum(total) FILTER (WHERE payment_status = 'paid' AND payment_method = 'zalopay'), 0)::bigint AS zalopay,
  count(*) FILTER (WHERE payment_status = 'paid' AND source = 'pos')::bigint                AS pos_orders,
  count(*) FILTER (WHERE payment_status = 'paid' AND source = 'app')::bigint                AS app_orders,
  count(*) FILTER (WHERE status = 'pending')::bigint                                        AS pending_app
FROM orders
WHERE store_id = $1 AND created_at >= $2 AND created_at < $3;

-- name: UpsertStoreItemAvailability :one
INSERT INTO store_menu_items (store_id, item_id, is_available)
VALUES ($1, $2, $3)
ON CONFLICT (store_id, item_id) DO UPDATE
SET is_available = EXCLUDED.is_available, updated_at = now()
RETURNING *;

-- name: ListStoreItemIDs :many
SELECT i.id, i.name, i.base_price, i.options, i.is_available,
       COALESCE(o.price_override, i.base_price)::bigint          AS price,
       (i.is_available AND COALESCE(o.is_available, true))::boolean AS store_available
FROM menu_items i
JOIN stores s ON s.id = $1 AND s.brand_id = i.brand_id
LEFT JOIN store_menu_items o ON o.item_id = i.id AND o.store_id = s.id
WHERE i.id = ANY($2::uuid[]);

-- name: CountOrdersByStore :one
SELECT count(*) FROM orders WHERE store_id = $1;
