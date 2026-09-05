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
