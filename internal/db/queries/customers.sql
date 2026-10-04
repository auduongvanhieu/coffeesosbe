-- name: GetCustomerByPhone :one
SELECT * FROM customers WHERE brand_id = $1 AND phone = $2;

-- name: GetCustomerByID :one
SELECT * FROM customers WHERE id = $1 AND brand_id = $2;

-- name: CreateCustomer :one
INSERT INTO customers (brand_id, phone, name, points)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: AddCustomerPoints :one
UPDATE customers SET points = points + $3, updated_at = now()
WHERE id = $1 AND brand_id = $2
RETURNING *;
