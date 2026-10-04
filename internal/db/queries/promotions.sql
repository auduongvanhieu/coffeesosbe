-- name: GetPromotionByCode :one
SELECT * FROM promotions
WHERE brand_id = $1 AND upper(code) = upper($2) AND is_active;

-- name: CreatePromotion :one
INSERT INTO promotions (brand_id, code, name, type, value, min_subtotal)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;
