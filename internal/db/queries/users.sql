-- name: GetUserByEmail :one
SELECT u.*, r.level AS role_level
FROM users u
JOIN roles r ON r.code = u.role_code
WHERE lower(u.email) = lower($1);

-- name: GetUserByID :one
SELECT u.*, r.level AS role_level
FROM users u
JOIN roles r ON r.code = u.role_code
WHERE u.id = $1;

-- name: CreateUser :one
INSERT INTO users (brand_id, store_id, role_code, email, phone, full_name, password_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListUsersByBrand :many
SELECT * FROM users
WHERE brand_id = $1
ORDER BY created_at;

-- name: CountUsersByRole :one
SELECT count(*) FROM users WHERE role_code = $1;

-- name: ListPinUsersByStore :many
-- Candidates for PIN login on a store terminal: staff pinned to the store plus
-- brand-level managers/owners of the same brand who have a PIN.
SELECT u.*, r.level AS role_level
FROM users u
JOIN roles r ON r.code = u.role_code
JOIN stores s ON s.id = $1
WHERE u.is_active AND u.pin_hash IS NOT NULL
  AND u.brand_id = s.brand_id
  AND (u.store_id = s.id OR u.store_id IS NULL);

-- name: SetUserPin :exec
UPDATE users SET pin_hash = $3, updated_at = now()
WHERE id = $1 AND brand_id = $2;

-- name: GetUserScope :one
SELECT b.name AS brand_name, b.logo_url AS brand_logo_url, s.name AS store_name
FROM users u
LEFT JOIN brands b ON b.id = u.brand_id
LEFT JOIN stores s ON s.id = u.store_id
WHERE u.id = $1;
