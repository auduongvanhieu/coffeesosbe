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
-- Deactivated accounts stay in the table for order history but are hidden here.
SELECT * FROM users
WHERE brand_id = $1 AND is_active
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

-- name: SetUserAvatar :one
UPDATE users SET avatar_url = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetUserAvatarInBrand :one
UPDATE users SET avatar_url = $3, updated_at = now()
WHERE id = $1 AND brand_id = $2
RETURNING *;

-- name: UpdateUserProfile :one
UPDATE users SET full_name = $2, email = $3, phone = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: CreatePasswordReset :one
INSERT INTO password_resets (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordReset :one
SELECT * FROM password_resets WHERE token_hash = $1;

-- name: UsePasswordReset :exec
UPDATE password_resets SET used_at = now() WHERE id = $1;

-- name: InvalidateUserResets :exec
UPDATE password_resets SET used_at = now()
WHERE user_id = $1 AND used_at IS NULL;

-- name: CountOrdersByUser :one
SELECT count(*) FROM orders WHERE created_by = $1;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1 AND brand_id = $2;

-- name: DeactivateUser :one
-- Keeps the row so past orders still show who rang them up, but frees the
-- email so the same address can be used for a new account.
UPDATE users
SET is_active = false,
    email = NULL,
    pin_hash = NULL,
    password_hash = NULL,
    full_name = full_name || ' (đã nghỉ)',
    updated_at = now()
WHERE id = $1 AND brand_id = $2
RETURNING *;
