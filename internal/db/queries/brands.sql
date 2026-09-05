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
