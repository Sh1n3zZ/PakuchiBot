package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("user already exists")
)

type User struct {
	ID        int64     `db:"id"`
	UserID    string    `db:"user_id"`
	Token     string    `db:"token"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, userID, token string) error {
	query := `
		INSERT INTO mgclub_users (user_id, token)
		VALUES (?, ?)
	`

	_, err := r.db.ExecContext(ctx, query, userID, token)
	if err != nil {
		return errors.Join(errors.New("failed to create user"), err)
	}

	return nil
}

func (r *UserRepository) Update(ctx context.Context, userID, token string) error {
	query := `
		UPDATE mgclub_users
		SET token = ?
		WHERE user_id = ?
	`

	result, err := r.db.ExecContext(ctx, query, token, userID)
	if err != nil {
		return errors.Join(errors.New("failed to update user"), err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Join(errors.New("failed to get affected rows"), err)
	}

	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (r *UserRepository) GetByUserID(ctx context.Context, userID string) (*User, error) {
	var user User
	query := `
		SELECT id, user_id, token, created_at, updated_at
		FROM mgclub_users
		WHERE user_id = ?
	`

	err := r.db.GetContext(ctx, &user, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, errors.Join(errors.New("failed to get user"), err)
	}

	return &user, nil
}

func (r *UserRepository) GetAllUsers(ctx context.Context) ([]User, error) {
	var users []User
	query := `
		SELECT id, user_id, token, created_at, updated_at
		FROM mgclub_users
	`

	err := r.db.SelectContext(ctx, &users, query)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get all users"), err)
	}

	return users, nil
}

func (r *UserRepository) Delete(ctx context.Context, userID string) error {
	query := `
		DELETE FROM mgclub_users
		WHERE user_id = ?
	`

	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return errors.Join(errors.New("failed to delete user"), err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Join(errors.New("failed to get affected rows"), err)
	}

	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}
