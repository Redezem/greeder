package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"testing"
)

func TestEnsureColumnAddAndExists(t *testing.T) {
	store, _ := newWritableStore(t)
	if err := ensureColumn(store.db, "articles", "base_url", "TEXT"); err != nil {
		t.Fatalf("ensureColumn existing error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS test_columns (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table error: %v", err)
	}
	if err := ensureColumn(store.db, "test_columns", "extra", "TEXT"); err != nil {
		t.Fatalf("ensureColumn add error: %v", err)
	}
	rows, err := store.db.Query(`PRAGMA table_info(test_columns)`)
	if err != nil {
		t.Fatalf("pragma error: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan error: %v", err)
		}
		if name == "extra" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected extra column")
	}
}

func TestEnsureColumnQueryError(t *testing.T) {
	store, _ := newWritableStore(t)
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if err := ensureColumn(store.db, "articles", "missing", "TEXT"); err == nil {
		t.Fatalf("expected ensureColumn query error")
	}
}

func TestEnsureColumnScanError(t *testing.T) {
	registerErrScanDriver()
	db, err := sql.Open("errscan", "")
	if err != nil {
		t.Fatalf("open errscan error: %v", err)
	}
	defer db.Close()
	if err := ensureColumn(db, "articles", "missing", "TEXT"); err == nil {
		t.Fatalf("expected ensureColumn scan error")
	}
}

func TestInitSchemaEnsureColumnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite error: %v", err)
	}
	defer db.Close()
	orig := ensureColumnFn
	call := 0
	ensureColumnFn = func(*sql.DB, string, string, string) error {
		call++
		if call == 2 {
			return errors.New("ensure column")
		}
		return nil
	}
	t.Cleanup(func() { ensureColumnFn = orig })
	if err := initSchema(db); err == nil {
		t.Fatalf("expected initSchema ensure column error")
	}
}

func TestInitSchemaEnsureColumnFirstError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite error: %v", err)
	}
	defer db.Close()
	orig := ensureColumnFn
	ensureColumnFn = func(*sql.DB, string, string, string) error {
		return errors.New("ensure column")
	}
	t.Cleanup(func() { ensureColumnFn = orig })
	if err := initSchema(db); err == nil {
		t.Fatalf("expected initSchema ensure column error")
	}
}

var errScanRegisterOnce sync.Once

func registerErrScanDriver() {
	errScanRegisterOnce.Do(func() {
		sql.Register("errscan", errScanDriver{})
	})
}

type errScanDriver struct{}

func (errScanDriver) Open(name string) (driver.Conn, error) {
	return &errScanConn{}, nil
}

type errScanConn struct{}

func (c *errScanConn) Prepare(query string) (driver.Stmt, error) {
	return &errScanStmt{}, nil
}

func (c *errScanConn) Close() error {
	return nil
}

func (c *errScanConn) Begin() (driver.Tx, error) {
	return &errScanTx{}, nil
}

func (c *errScanConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return &errScanRows{}, nil
}

type errScanStmt struct{}

func (s *errScanStmt) Close() error {
	return nil
}

func (s *errScanStmt) NumInput() int {
	return 0
}

func (s *errScanStmt) Exec(args []driver.Value) (driver.Result, error) {
	return errScanResult(0), nil
}

func (s *errScanStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &errScanRows{}, nil
}

type errScanTx struct{}

func (t *errScanTx) Commit() error   { return nil }
func (t *errScanTx) Rollback() error { return nil }

type errScanRows struct {
	done bool
}

func (r *errScanRows) Columns() []string {
	return []string{"only"}
}

func (r *errScanRows) Close() error { return nil }

func (r *errScanRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = "value"
	return nil
}

type errScanResult int64

func (r errScanResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r errScanResult) RowsAffected() (int64, error) { return int64(r), nil }
