package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

var (
	db     *sql.DB
	sqlxDB *sqlx.DB
)

func InitDB(dbPath string) error {
	if strings.HasPrefix(dbPath, "~") {
		home, err := getHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user directory: %w", err)
		}
		dbPath = filepath.Join(home, dbPath[1:])
	}

	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	dbPath = absPath

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	if err := checkDirPermissions(dbDir); err != nil {
		return fmt.Errorf("failed to check directory permissions: %w", err)
	}

	var dbErr error
	db, dbErr = sql.Open("sqlite", fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&cache=shared", dbPath))
	if dbErr != nil {
		return fmt.Errorf("failed to open database connection: %w", dbErr)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to test database connection: %w", err)
	}

	sqlxDB = sqlx.NewDb(db, "sqlite")

	if err := migrate(); err != nil {
		db.Close()
		return fmt.Errorf("failed on database migration: %w", err)
	}

	return nil
}

func GetDB() *sqlx.DB {
	return sqlxDB
}

func CloseDB() error {
	if sqlxDB != nil {
		sqlxDB.Close()
	}
	if db != nil {
		return db.Close()
	}
	return nil
}

func migrate() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create migration log table: %w", err)
	}

	appliedVersions := make(map[string]bool)
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("failed to read migration version: %w", err)
		}
		appliedVersions[version] = true
	}

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}
	sort.Strings(files)

	for _, file := range files {
		version := filepath.Base(file[:len(file)-4])
		if appliedVersions[version] {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", file, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration version: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return nil
}

func checkDirPermissions(dir string) error {
	testFile := filepath.Join(dir, ".test_write_permission")
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create test file in directory: %w", err)
	}
	f.Close()

	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("failed to remove test file: %w", err)
	}

	return nil
}

func getHomeDir() (string, error) {
	if runtime.GOOS == "windows" {
		home := os.Getenv("USERPROFILE")
		if home == "" {
			home = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		}
		if home == "" {
			return "", fmt.Errorf("failed to get user home directory")
		}
		return home, nil
	}

	home := os.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("failed to get user home directory")
	}
	return home, nil
}
