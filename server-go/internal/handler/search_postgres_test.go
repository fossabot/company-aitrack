package handler_test

// search_postgres_test.go exercises the isPostgres=true code paths in
// SearchHandler.Search and SimilarHandler.Similar using an inline
// database/sql/driver mock — no external dependencies required.

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aitrack/server/internal/handler"
)

// ─── minimal mock driver ────────────────────────────────────────────────────

// mockDriver is a database/sql/driver.Driver that returns a fixed set of rows
// (or an error) for every QueryContext call.
type mockDriver struct {
	rows [][]driver.Value // nil means return a query error
	err  error            // non-nil: QueryContext returns this error
	rowsErr error         // non-nil: Rows.Err() returns this after iteration
	scanErr bool          // if true, first Scan returns driver.ErrBadConn
}

var (
	mockDriverMu    sync.Mutex
	mockDriverStore = map[string]*mockDriver{}
	mockDriverOnce  sync.Once
)

func registerMockDriver() {
	mockDriverOnce.Do(func() {
		sql.Register("mockpg", &mockDriverDispatch{})
	})
}

// mockDriverDispatch implements driver.Driver and looks up the per-DSN mock.
type mockDriverDispatch struct{}

func (d *mockDriverDispatch) Open(name string) (driver.Conn, error) {
	mockDriverMu.Lock()
	m := mockDriverStore[name]
	mockDriverMu.Unlock()
	if m == nil {
		return nil, errors.New("no mock registered for dsn " + name)
	}
	return &mockConn{mock: m}, nil
}

// mockConn implements driver.Conn.
type mockConn struct{ mock *mockDriver }

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{mock: c.mock}, nil
}
func (c *mockConn) Close() error  { return nil }
func (c *mockConn) Begin() (driver.Tx, error) {
	return &mockTx{}, nil
}

type mockTx struct{}

func (t *mockTx) Commit() error   { return nil }
func (t *mockTx) Rollback() error { return nil }

// mockStmt implements driver.Stmt and driver.StmtQueryContext.
type mockStmt struct{ mock *mockDriver }

func (s *mockStmt) Close() error                                    { return nil }
func (s *mockStmt) NumInput() int                                   { return -1 }
func (s *mockStmt) Exec(args []driver.Value) (driver.Result, error) { return nil, nil }
func (s *mockStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.mock.err != nil {
		return nil, s.mock.err
	}
	return &mockRows{mock: s.mock, pos: 0}, nil
}

// mockRows implements driver.Rows.
type mockRows struct {
	mock *mockDriver
	pos  int
}

func (r *mockRows) Columns() []string {
	// 9 columns matching the Search / Similar SELECT lists
	return []string{"id", "token_key", "repo_url", "file_path", "diff_hunk",
		"added_lines", "removed_lines", "received_at", "score"}
}

func (r *mockRows) Close() error { return nil }

func (r *mockRows) Next(dest []driver.Value) error {
	if r.mock.scanErr && r.pos == 0 {
		return driver.ErrBadConn
	}
	if r.mock.rowsErr != nil && r.pos == len(r.mock.rows) {
		// signal rows.Err() after all rows are read — we do this by returning
		// the error on the first extra Next() call.
		return r.mock.rowsErr
	}
	if r.pos >= len(r.mock.rows) {
		return io.EOF
	}
	row := r.mock.rows[r.pos]
	r.pos++
	copy(dest, row)
	return nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

// openMockDB opens a *sql.DB backed by the mock driver with the given DSN key
// and registers the supplied mock for that key.
func openMockDB(t *testing.T, dsn string, m *mockDriver) *sql.DB {
	t.Helper()
	registerMockDriver()
	mockDriverMu.Lock()
	mockDriverStore[dsn] = m
	mockDriverMu.Unlock()
	t.Cleanup(func() {
		mockDriverMu.Lock()
		delete(mockDriverStore, dsn)
		mockDriverMu.Unlock()
	})
	db, err := sql.Open("mockpg", dsn)
	if err != nil {
		t.Fatalf("open mock db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newSearchPostgresRouter wires Search + Similar handlers with isPostgres=true
// and the given mock DB.
func newSearchPostgresRouter(db *sql.DB, adminKey string) http.Handler {
	searchH := handler.NewSearchHandler(db, adminKey, true)
	similarH := handler.NewSimilarHandler(db, adminKey, true)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/ai-track/edits/search", searchH.Search)
	mux.HandleFunc("/api/v1/ai-track/edits/similar", similarH.Similar)
	return mux
}

// goodSearchRow returns a driver.Value row matching the 9-column SELECT in Search.
func goodSearchRow() []driver.Value {
	return []driver.Value{
		int64(1),           // id
		"tok-abc",          // token_key
		"https://repo",     // repo_url
		"main.go",          // file_path
		"- old\n+ new\n",   // diff_hunk
		int64(5),           // added_lines
		int64(2),           // removed_lines
		time.Now().UTC(),   // received_at
		float64(1.23),      // score / distance
	}
}

const adminKey = "test-admin-key-pg"

// ─── Search (isPostgres=true) tests ─────────────────────────────────────────

// TestSearch_Postgres_HappyPath: returns 200 with hits slice.
func TestSearch_Postgres_HappyPath(t *testing.T) {
	db := openMockDB(t, "search-happy", &mockDriver{rows: [][]driver.Value{goodSearchRow()}})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-track/edits/search?q=hello&limit=5", nil)
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	decodeJSON(t, w, &resp)
	hits, ok := resp["hits"].([]interface{})
	if !ok || len(hits) != 1 {
		t.Errorf("expected 1 hit, got %v", resp["hits"])
	}
}

// TestSearch_Postgres_WithFilters: token_key + repo filters included.
func TestSearch_Postgres_WithFilters(t *testing.T) {
	db := openMockDB(t, "search-filters", &mockDriver{rows: [][]driver.Value{goodSearchRow()}})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ai-track/edits/search?q=fix&token_key=tok1&repo=myrepo", nil)
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)
}

// TestSearch_Postgres_EmptyResult: zero rows → hits:[] with total=0.
func TestSearch_Postgres_EmptyResult(t *testing.T) {
	db := openMockDB(t, "search-empty", &mockDriver{rows: [][]driver.Value{}})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-track/edits/search?q=nothing", nil)
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	decodeJSON(t, w, &resp)
	if total, _ := resp["total"].(float64); total != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

// TestSearch_Postgres_QueryError: DB returns error → 500.
func TestSearch_Postgres_QueryError(t *testing.T) {
	db := openMockDB(t, "search-qerr", &mockDriver{err: errors.New("db down")})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-track/edits/search?q=foo", nil)
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusInternalServerError)
}

// TestSearch_Postgres_ScanError: rows.Scan fails → 500.
func TestSearch_Postgres_ScanError(t *testing.T) {
	db := openMockDB(t, "search-scanerr", &mockDriver{
		rows:    [][]driver.Value{goodSearchRow()},
		scanErr: true,
	})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-track/edits/search?q=foo", nil)
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusInternalServerError)
}

// TestSearch_Postgres_RowsErr: rows.Err() returns error after iteration → 500.
func TestSearch_Postgres_RowsErr(t *testing.T) {
	db := openMockDB(t, "search-rowserr", &mockDriver{
		rows:    [][]driver.Value{},
		rowsErr: errors.New("network reset"),
	})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-track/edits/search?q=foo", nil)
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusInternalServerError)
}

// ─── Similar (isPostgres=true) tests ─────────────────────────────────────────

func valid384Embedding() []float32 {
	emb := make([]float32, 384)
	for i := range emb {
		emb[i] = float32(i) * 0.0026
	}
	return emb
}

func similarBody(t *testing.T, extra map[string]interface{}) []byte {
	t.Helper()
	payload := map[string]interface{}{
		"embedding": valid384Embedding(),
		"limit":     10,
	}
	for k, v := range extra {
		payload[k] = v
	}
	b, _ := json.Marshal(payload)
	return b
}

// TestSimilar_Postgres_HappyPath: 200 with hits.
func TestSimilar_Postgres_HappyPath(t *testing.T) {
	db := openMockDB(t, "similar-happy", &mockDriver{rows: [][]driver.Value{goodSearchRow()}})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai-track/edits/similar",
		bytes.NewReader(similarBody(t, nil)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	decodeJSON(t, w, &resp)
	hits, ok := resp["hits"].([]interface{})
	if !ok || len(hits) != 1 {
		t.Errorf("expected 1 hit, got %v", resp["hits"])
	}
}

// TestSimilar_Postgres_WithFilters: token_key + repo included in body.
func TestSimilar_Postgres_WithFilters(t *testing.T) {
	db := openMockDB(t, "similar-filters", &mockDriver{rows: [][]driver.Value{goodSearchRow()}})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai-track/edits/similar",
		bytes.NewReader(similarBody(t, map[string]interface{}{
			"token_key": "tok-xyz",
			"repo":      "my-repo",
		})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)
}

// TestSimilar_Postgres_LimitZero: limit<=0 → default 20 (no error).
func TestSimilar_Postgres_LimitZero(t *testing.T) {
	db := openMockDB(t, "similar-limitzero", &mockDriver{rows: [][]driver.Value{}})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai-track/edits/similar",
		bytes.NewReader(similarBody(t, map[string]interface{}{"limit": 0})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)
}

// TestSimilar_Postgres_LimitOverMax: limit>100 → clamped to 100.
func TestSimilar_Postgres_LimitOverMax(t *testing.T) {
	db := openMockDB(t, "similar-limitmax", &mockDriver{rows: [][]driver.Value{}})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai-track/edits/similar",
		bytes.NewReader(similarBody(t, map[string]interface{}{"limit": 9999})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)
}

// TestSimilar_Postgres_QueryError: DB error → 500.
func TestSimilar_Postgres_QueryError(t *testing.T) {
	db := openMockDB(t, "similar-qerr", &mockDriver{err: errors.New("db gone")})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai-track/edits/similar",
		bytes.NewReader(similarBody(t, nil)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusInternalServerError)
}

// TestSimilar_Postgres_ScanError: scan fails → 500.
func TestSimilar_Postgres_ScanError(t *testing.T) {
	db := openMockDB(t, "similar-scanerr", &mockDriver{
		rows:    [][]driver.Value{goodSearchRow()},
		scanErr: true,
	})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai-track/edits/similar",
		bytes.NewReader(similarBody(t, nil)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusInternalServerError)
}

// TestSimilar_Postgres_RowsErr: rows.Err() → 500.
func TestSimilar_Postgres_RowsErr(t *testing.T) {
	db := openMockDB(t, "similar-rowserr", &mockDriver{
		rows:    [][]driver.Value{},
		rowsErr: errors.New("broken pipe"),
	})
	router := newSearchPostgresRouter(db, adminKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai-track/edits/similar",
		bytes.NewReader(similarBody(t, nil)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusInternalServerError)
}
