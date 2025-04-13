package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type NotifySetting struct {
	ID        int64     `db:"id"`
	UserID    string    `db:"user_id"`
	GroupID   *int64    `db:"group_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type NotifyRepository struct {
	db *sqlx.DB
}

func NewNotifyRepository(db *sqlx.DB) *NotifyRepository {
	return &NotifyRepository{db: db}
}

func (r *NotifyRepository) UpsertSetting(ctx context.Context, userID string, groupID *int64) error {
	query := `
		INSERT INTO mgclub_notify_settings (user_id, group_id)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			group_id = excluded.group_id,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := r.db.ExecContext(ctx, query, userID, groupID)
	if err != nil {
		return errors.Join(errors.New("failed to upsert notify setting"), err)
	}

	return nil
}

func (r *NotifyRepository) GetSetting(ctx context.Context, userID string) (*NotifySetting, error) {
	var setting NotifySetting
	query := `
		SELECT id, user_id, group_id, created_at, updated_at
		FROM mgclub_notify_settings
		WHERE user_id = ?
	`

	err := r.db.GetContext(ctx, &setting, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, errors.Join(errors.New("failed to get notify setting"), err)
	}

	return &setting, nil
}

func (r *NotifyRepository) GetAllSettings(ctx context.Context) ([]NotifySetting, error) {
	var settings []NotifySetting
	query := `
		SELECT id, user_id, group_id, created_at, updated_at
		FROM mgclub_notify_settings
	`

	err := r.db.SelectContext(ctx, &settings, query)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get all notify settings"), err)
	}

	return settings, nil
}
