-- name: ListCategoriesByBrand :many
SELECT * FROM menu_categories
WHERE brand_id = $1
ORDER BY sort_order, name;

-- name: CreateCategory :one
INSERT INTO menu_categories (brand_id, name, sort_order)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListItemsByBrand :many
SELECT * FROM menu_items
WHERE brand_id = $1
ORDER BY category_id, sort_order, name;

-- name: GetItemByID :one
SELECT * FROM menu_items
WHERE id = $1 AND brand_id = $2;

-- name: CreateItem :one
INSERT INTO menu_items (brand_id, category_id, name, description, image_url, base_price, options, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: SetItemAvailability :one
UPDATE menu_items
SET is_available = $3, updated_at = now()
WHERE id = $1 AND brand_id = $2
RETURNING *;

-- name: ListCategoriesByStore :many
SELECT c.*
FROM menu_categories c
JOIN stores s ON s.brand_id = c.brand_id
WHERE s.id = $1 AND c.is_active
ORDER BY c.sort_order, c.name;

-- name: ListStoreMenu :many
SELECT i.*,
       COALESCE(o.price_override, i.base_price)::bigint          AS price,
       (i.is_available AND COALESCE(o.is_available, true))::boolean AS store_available
FROM menu_items i
JOIN stores s ON s.id = $1 AND s.brand_id = i.brand_id
LEFT JOIN store_menu_items o ON o.item_id = i.id AND o.store_id = s.id
ORDER BY i.category_id, i.sort_order, i.name;

-- name: UpdateCategory :one
UPDATE menu_categories
SET name = $3, sort_order = $4, is_active = $5, updated_at = now()
WHERE id = $1 AND brand_id = $2
RETURNING *;

-- name: UpdateItem :one
UPDATE menu_items
SET category_id = $3, name = $4, description = $5, image_url = $6,
    base_price = $7, options = $8, sort_order = $9, updated_at = now()
WHERE id = $1 AND brand_id = $2
RETURNING *;
