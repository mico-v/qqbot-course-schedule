// Package store persists course schedules in SQLite using the schema inherited
// from the AstrBot plugin (members, events, day overrides) plus a KV table.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"

	_ "modernc.org/sqlite"
)

// Store is the SQLite-backed implementation of schedule.Storage.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

var (
	_ schedule.ServiceStorage = (*Store)(nil)
	_ admin.Storage           = (*Store)(nil)
)

// Open opens (and creates) the database at path.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("数据库路径不能为空")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// One connection avoids SQLITE_BUSY between pooled writers.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 30000",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("设置 %s 失败: %w", pragma, err)
		}
	}
	if err := backupBeforeV7(db, path); err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func backupBeforeV7(db *sql.DB, path string) error {
	var version string
	err := db.QueryRow(`SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		// A missing metadata table means this is a fresh or pre-metadata
		// database; migration will either create it or report the setup error.
		var table string
		checkErr := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'metadata'`,
		).Scan(&table)
		if errors.Is(checkErr, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("读取迁移前 schema 版本失败: %w", err)
	}
	versionNumber, parseErr := strconv.Atoi(version)
	if parseErr != nil {
		return fmt.Errorf("迁移前 schema 版本 %q 无效: %w", version, parseErr)
	}
	if versionNumber >= schemaVersion {
		return nil
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		return nil
	}
	backupPath := fmt.Sprintf(
		"%s.v%s.%s.bak",
		path,
		version,
		time.Now().UTC().Format("20060102T150405.000000000"),
	)
	escapedPath := strings.ReplaceAll(backupPath, "'", "''")
	if _, err := db.Exec(`VACUUM INTO '` + escapedPath + `'`); err != nil {
		return fmt.Errorf("创建迁移前备份失败: %w", err)
	}
	return nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator +
		"_pragma=foreign_keys(1)&" +
		"_pragma=busy_timeout(30000)&" +
		"_txlock=immediate"
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
