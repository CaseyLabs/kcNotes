// TEACHING NOTES:
// SQLite store files implement persistence behind focused methods.
// In Go, `database/sql` provides a portable API over concrete drivers.
// Useful Go concepts to notice:
// 1. Use context-aware DB calls (`QueryContext`, `ExecContext`).
// 2. Scan DB rows into strongly-typed structs.
// 3. Keep SQL close to call sites for readability unless reuse demands abstraction.
// 4. Wrap low-level errors with operation context for better debugging.
// 5. Transactions should stay short and avoid external IO inside.
package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	libsql "github.com/tursodatabase/go-libsql"
)

type DBMode string

const (
	DBModeLocal   DBMode = "local"
	DBModeRemote  DBMode = "remote"
	DBModeReplica DBMode = "replica"
)

type OpenConfig struct {
	Mode                  DBMode
	DBPath                string
	DatabaseURL           string
	DatabaseAuthToken     string
	ReplicaSyncInterval   time.Duration
	ReplicaReadYourWrites bool
}

type Connection struct {
	DB    *sql.DB
	close func() error
}

// Close explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (c *Connection) Close() error {
	if c == nil {
		return nil
	}
	if c.close != nil {
		return c.close()
	}
	if c.DB != nil {
		return c.DB.Close()
	}
	return nil
}

// Open explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func Open(cfg OpenConfig) (*Connection, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = DBModeLocal
	}
	switch mode {
	case DBModeLocal:
		return openLocal(cfg.DBPath)
	case DBModeRemote:
		return openRemote(cfg.DatabaseURL, cfg.DatabaseAuthToken)
	case DBModeReplica:
		return openReplica(cfg)
	default:
		return nil, fmt.Errorf("unsupported db mode %q", mode)
	}
}

// openLocal explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func openLocal(path string) (*Connection, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s", path)
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open local sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)
	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&journalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set journal_mode: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}
	var busyTimeout int
	if err := db.QueryRow(`PRAGMA busy_timeout=5000`).Scan(&busyTimeout); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping local sqlite: %w", err)
	}
	return &Connection{DB: db, close: db.Close}, nil
}

// openRemote explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func openRemote(databaseURL, authToken string) (*Connection, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database url is required for remote mode")
	}
	dsn, err := buildRemoteDSN(databaseURL, authToken)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open remote libsql: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping remote libsql: %w", err)
	}
	return &Connection{DB: db, close: db.Close}, nil
}

// openReplica explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func openReplica(cfg OpenConfig) (*Connection, error) {
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("db path is required for replica mode")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database url is required for replica mode")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir replica db dir: %w", err)
	}

	opts := make([]libsql.Option, 0, 3)
	if cfg.DatabaseAuthToken != "" {
		opts = append(opts, libsql.WithAuthToken(cfg.DatabaseAuthToken))
	}
	opts = append(opts, libsql.WithReadYourWrites(cfg.ReplicaReadYourWrites))
	if cfg.ReplicaSyncInterval > 0 {
		opts = append(opts, libsql.WithSyncInterval(cfg.ReplicaSyncInterval))
	}

	connector, err := libsql.NewEmbeddedReplicaConnector(cfg.DBPath, cfg.DatabaseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("create replica connector: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		_ = connector.Close()
		return nil, fmt.Errorf("ping replica libsql: %w", err)
	}

	return &Connection{
		DB: db,
		close: func() error {
			dbErr := db.Close()
			connErr := connector.Close()
			if dbErr != nil {
				return dbErr
			}
			return connErr
		},
	}, nil
}

// buildRemoteDSN explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func buildRemoteDSN(databaseURL, authToken string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}
	if parsed.Scheme == "" {
		return "", fmt.Errorf("database url must include scheme")
	}
	if authToken != "" {
		q := parsed.Query()
		if q.Get("authToken") == "" {
			q.Set("authToken", authToken)
		}
		parsed.RawQuery = q.Encode()
	}
	return parsed.String(), nil
}
