package auth

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jackc/pgx/v5"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) CreateUser(email, passwordHash string) error {
	query := `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
	`

	_, err := r.db.Exec(
		context.Background(),
		query,
		email,
		passwordHash,
	)

	return err
}

func (r *Repository) GetUserByEmail(email string) (*User, error) {
	query := `
		SELECT id, email, password_hash
		FROM users
		WHERE email = $1
	`

	var user User

	err := r.db.QueryRow(
		context.Background(),
		query,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
	)

	if err != nil {
		return nil, err
	}

	return &user, nil
}
func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, email, password_hash, name, status, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	var u User
	var name sql.NullString
	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &name, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	if name.Valid {
		u.Name = &name.String
	}
	return &u, nil
}
