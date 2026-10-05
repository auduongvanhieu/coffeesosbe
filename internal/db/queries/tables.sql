-- name: ListStoreTables :many
SELECT * FROM store_tables
WHERE store_id = $1 AND is_active
ORDER BY sort_order, name;

-- name: ListActiveTableOrders :many
-- One row per table that currently has an unfinished order on it. Joined in
-- Go rather than with a LEFT JOIN LATERAL so every column stays non-null.
SELECT DISTINCT ON (o.table_id)
       o.table_id, o.id, o.number, o.status, o.payment_status, o.total, o.created_at,
       COALESCE((SELECT sum(oi.quantity) FROM order_items oi WHERE oi.order_id = o.id), 0)::bigint AS item_count
FROM orders o
WHERE o.store_id = $1
  AND o.table_id IS NOT NULL
  AND o.status IN ('open', 'preparing', 'ready')
ORDER BY o.table_id, o.created_at DESC;

-- name: GetStoreTable :one
SELECT * FROM store_tables WHERE id = $1 AND store_id = $2;

-- name: CreateStoreTable :one
INSERT INTO store_tables (store_id, name, zone, seats, sort_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateStoreTable :one
UPDATE store_tables
SET name = $3, zone = $4, seats = $5, sort_order = $6, is_active = $7, updated_at = now()
WHERE id = $1 AND store_id = $2
RETURNING *;

-- name: DeleteStoreTable :exec
UPDATE store_tables SET is_active = false, updated_at = now()
WHERE id = $1 AND store_id = $2;

-- name: CountStoreTables :one
SELECT count(*) FROM store_tables WHERE store_id = $1;
