-- name: CreateBrand :one
INSERT INTO brands (slug, name, slogan, logo_url, theme)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetBrandByID :one
SELECT * FROM brands WHERE id = $1;

-- name: GetBrandBySlug :one
SELECT * FROM brands WHERE slug = $1;

-- name: ListBrands :many
SELECT * FROM brands ORDER BY created_at;

-- name: CreateStore :one
INSERT INTO stores (brand_id, code, name, address, phone)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetStoreByID :one
SELECT * FROM stores WHERE id = $1;

-- name: ListStoresByBrand :many
SELECT * FROM stores WHERE brand_id = $1 ORDER BY created_at;

-- name: UpdateBrand :one
UPDATE brands
SET name = $2, slogan = $3, logo_url = $4, theme = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetBrandStats :one
SELECT
  (SELECT count(*) FROM stores s          WHERE s.brand_id = $1 AND s.is_active)::bigint    AS stores,
  (SELECT count(*) FROM users u           WHERE u.brand_id = $1 AND u.is_active)::bigint    AS users,
  (SELECT count(*) FROM menu_categories c WHERE c.brand_id = $1 AND c.is_active)::bigint    AS categories,
  (SELECT count(*) FROM menu_items i      WHERE i.brand_id = $1)::bigint                    AS items,
  (SELECT count(*) FROM menu_items i      WHERE i.brand_id = $1 AND i.is_available)::bigint AS available_items;

-- name: UpdateStoreBank :one
UPDATE stores
SET bank_bin = $2, bank_code = $3, bank_account = $4, bank_holder = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: NextOrderNo :one
UPDATE stores SET next_order_no = next_order_no + 1
WHERE id = $1
RETURNING (next_order_no - 1)::integer AS order_no;
