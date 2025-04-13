package storage

import (
	"database/sql"
	"fmt"
)

func GetUserLuck(userID string, day string) (int, bool, error) {
	var value int
	err := db.QueryRow("SELECT value FROM user_luck WHERE user_id = ? AND day = ?", userID, day).Scan(&value)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to get user luck: %w", err)
	}
	return value, true, nil
}

func RecordUserLuck(userID string, day string, value int) error {
	_, err := db.Exec(
		"INSERT INTO user_luck (user_id, day, value) VALUES (?, ?, ?)",
		userID, day, value,
	)
	if err != nil {
		return fmt.Errorf("failed to record user luck: %w", err)
	}
	return nil
}
