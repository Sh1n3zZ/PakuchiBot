package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

type SignStatus int

const (
	SignStatusPending SignStatus = iota
	SignStatusSuccess
	SignStatusFailed
)

type SignRecord struct {
	ID          int64      `db:"id"`
	UserID      string     `db:"user_id"`
	SignDate    time.Time  `db:"sign_date"`
	Status      SignStatus `db:"status"`
	RetryCount  int        `db:"retry_count"`
	LastRetryAt *time.Time `db:"last_retry_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

type SignRepository struct {
	db *sqlx.DB
}

func NewSignRepository(db *sqlx.DB) *SignRepository {
	return &SignRepository{db: db}
}

func (r *SignRepository) InitDailyRecords(ctx context.Context, userIDs []string) error {
	today := time.Now().Format("2006-01-02")
	query := `
		INSERT INTO sign_records (user_id, sign_date)
		VALUES (?, ?)
		ON CONFLICT(user_id, sign_date) DO NOTHING
	`

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, userID := range userIDs {
		if _, err := stmt.ExecContext(ctx, userID, today); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *SignRepository) GetPendingRecords(ctx context.Context, maxRetries int) ([]SignRecord, error) {
	query := `
		SELECT id, user_id, sign_date, status, retry_count, last_retry_at, created_at, updated_at
		FROM sign_records
		WHERE sign_date = DATE('now', 'localtime')
		AND status != ?
		AND retry_count < ?
		AND (last_retry_at IS NULL OR last_retry_at < datetime('now', '-5 minutes'))
	`

	var records []SignRecord
	err := r.db.SelectContext(ctx, &records, query, SignStatusSuccess, maxRetries)
	if err != nil {
		return nil, err
	}

	return records, nil
}

func (r *SignRepository) UpdateStatus(ctx context.Context, id int64, status SignStatus) error {
	query := `
		UPDATE sign_records
		SET status = ?,
			retry_count = retry_count + 1,
			last_retry_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}
