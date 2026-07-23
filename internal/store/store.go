package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ganjar/ecorouter/internal/config"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = config.DBPath()
	}
	if err := config.EnsureDirs(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS tokens (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  hash TEXT NOT NULL,
  scope_route TEXT NOT NULL DEFAULT '',
  scope_models TEXT NOT NULL DEFAULT '',
  rate TEXT NOT NULL DEFAULT '',
  expires_at TEXT,
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  last_ip TEXT NOT NULL DEFAULT '',
  revoked INTEGER NOT NULL DEFAULT 0,
  daily_cap REAL NOT NULL DEFAULT 0,
  max_concurrent INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS activity (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT NOT NULL,
  token_id TEXT NOT NULL DEFAULT '',
  token_label TEXT NOT NULL DEFAULT '',
  src_ip TEXT NOT NULL DEFAULT '',
  route TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  via TEXT NOT NULL DEFAULT '',
  tokens_in INTEGER NOT NULL DEFAULT 0,
  tokens_out INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 0,
  cost_estimate REAL,
  error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS audit (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT NOT NULL,
  event TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  src_ip TEXT NOT NULL DEFAULT '',
  token_id TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_activity_ts ON activity(ts);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit(ts);
CREATE INDEX IF NOT EXISTS idx_tokens_revoked ON tokens(revoked);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Additive migrations for existing DBs created before these columns.
	for _, col := range []string{
		`ALTER TABLE tokens ADD COLUMN daily_cap REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE tokens ADD COLUMN max_concurrent INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = s.db.Exec(col)
	}
	return nil
}

// --- Tokens ---

type Token struct {
	ID            string     `json:"id"`
	Label         string     `json:"label"`
	Hash          string     `json:"-"`
	ScopeRoute    string     `json:"scope_route,omitempty"`
	ScopeModels   []string   `json:"scope_models,omitempty"`
	Rate          string     `json:"rate,omitempty"`
	DailyCap      float64    `json:"daily_cap,omitempty"`      // USD; 0 = disabled
	MaxConcurrent int        `json:"max_concurrent,omitempty"` // 0 = unlimited
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	LastIP        string     `json:"last_ip,omitempty"`
	Revoked       bool       `json:"revoked"`
}

func (s *Store) InsertToken(t *Token) error {
	models := strings.Join(t.ScopeModels, ",")
	var exp, lastUsed sql.NullString
	if t.ExpiresAt != nil {
		exp = sql.NullString{String: t.ExpiresAt.UTC().Format(time.RFC3339), Valid: true}
	}
	if t.LastUsedAt != nil {
		lastUsed = sql.NullString{String: t.LastUsedAt.UTC().Format(time.RFC3339), Valid: true}
	}
	rev := 0
	if t.Revoked {
		rev = 1
	}
	_, err := s.db.Exec(`
INSERT INTO tokens (id, label, hash, scope_route, scope_models, rate, expires_at, created_at, last_used_at, last_ip, revoked, daily_cap, max_concurrent)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Label, t.Hash, t.ScopeRoute, models, t.Rate, exp,
		t.CreatedAt.UTC().Format(time.RFC3339), lastUsed, t.LastIP, rev,
		t.DailyCap, t.MaxConcurrent,
	)
	return err
}

func (s *Store) UpdateTokenHash(id, hash string) error {
	_, err := s.db.Exec(`UPDATE tokens SET hash = ?, revoked = 0 WHERE id = ?`, hash, id)
	return err
}

func (s *Store) RevokeToken(id string) error {
	res, err := s.db.Exec(`UPDATE tokens SET revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token %s not found", id)
	}
	return nil
}

func (s *Store) UpdateTokenScope(id, route string, models []string) error {
	_, err := s.db.Exec(`UPDATE tokens SET scope_route = ?, scope_models = ? WHERE id = ?`,
		route, strings.Join(models, ","), id)
	return err
}

func (s *Store) TouchToken(id, ip string) error {
	_, err := s.db.Exec(`UPDATE tokens SET last_used_at = ?, last_ip = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), ip, id)
	return err
}

const tokenSelect = `SELECT id, label, hash, scope_route, scope_models, rate, expires_at, created_at, last_used_at, last_ip, revoked, daily_cap, max_concurrent FROM tokens`

func (s *Store) GetToken(id string) (*Token, error) {
	row := s.db.QueryRow(tokenSelect+` WHERE id = ?`, id)
	return scanToken(row)
}

func (s *Store) ListTokens() ([]Token, error) {
	rows, err := s.db.Query(tokenSelect + ` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		t, err := scanTokenRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// FindTokenByPlaintext verifies against all non-revoked tokens.
// For small operator deployments this is fine; v2 can add a lookup prefix.
func (s *Store) FindTokenByPlaintext(plaintext string, verify func(plain, hash string) (bool, error)) (*Token, error) {
	rows, err := s.db.Query(tokenSelect + ` WHERE revoked = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTokenRows(rows)
		if err != nil {
			return nil, err
		}
		ok, err := verify(plaintext, t.Hash)
		if err != nil {
			continue
		}
		if ok {
			if t.ExpiresAt != nil && time.Now().UTC().After(*t.ExpiresAt) {
				return nil, fmt.Errorf("token expired")
			}
			return t, nil
		}
	}
	return nil, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanToken(row scannable) (*Token, error) {
	var t Token
	var models string
	var exp, lastUsed sql.NullString
	var rev int
	var created string
	if err := row.Scan(&t.ID, &t.Label, &t.Hash, &t.ScopeRoute, &models, &t.Rate, &exp, &created, &lastUsed, &t.LastIP, &rev, &t.DailyCap, &t.MaxConcurrent); err != nil {
		return nil, err
	}
	t.ScopeModels = splitCSV(models)
	t.Revoked = rev != 0
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	if exp.Valid {
		tm, _ := time.Parse(time.RFC3339, exp.String)
		t.ExpiresAt = &tm
	}
	if lastUsed.Valid {
		tm, _ := time.Parse(time.RFC3339, lastUsed.String)
		t.LastUsedAt = &tm
	}
	return &t, nil
}

// DailySpend returns summed cost_estimate for a token (or all tokens if tokenID=="") since UTC midnight.
func (s *Store) DailySpend(tokenID string) (float64, error) {
	dayStart := time.Now().UTC().Truncate(24 * time.Hour).Format(time.RFC3339Nano)
	q := `SELECT COALESCE(SUM(cost_estimate), 0) FROM activity WHERE ts >= ? AND cost_estimate IS NOT NULL`
	args := []any{dayStart}
	if tokenID != "" {
		q += ` AND token_id = ?`
		args = append(args, tokenID)
	}
	var sum float64
	err := s.db.QueryRow(q, args...).Scan(&sum)
	return sum, err
}

func scanTokenRows(rows *sql.Rows) (*Token, error) {
	return scanToken(rows)
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// --- Activity ---

type Activity struct {
	ID           int64    `json:"id"`
	TS           time.Time `json:"ts"`
	TokenID      string   `json:"token_id"`
	TokenLabel   string   `json:"token_label"`
	SrcIP        string   `json:"src_ip"`
	Route        string   `json:"route"`
	Model        string   `json:"model"`
	Provider     string   `json:"provider"`
	Via          string   `json:"via,omitempty"`
	TokensIn     int      `json:"tokens_in"`
	TokensOut    int      `json:"tokens_out"`
	LatencyMs    int64    `json:"latency_ms"`
	Status       int      `json:"status"`
	CostEstimate *float64 `json:"cost_estimate,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func (s *Store) InsertActivity(a *Activity) error {
	var cost sql.NullFloat64
	if a.CostEstimate != nil {
		cost = sql.NullFloat64{Float64: *a.CostEstimate, Valid: true}
	}
	_, err := s.db.Exec(`
INSERT INTO activity (ts, token_id, token_label, src_ip, route, model, provider, via, tokens_in, tokens_out, latency_ms, status, cost_estimate, error)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TS.UTC().Format(time.RFC3339Nano), a.TokenID, a.TokenLabel, a.SrcIP, a.Route, a.Model, a.Provider, a.Via,
		a.TokensIn, a.TokensOut, a.LatencyMs, a.Status, cost, a.Error,
	)
	return err
}

func (s *Store) ListActivity(since time.Time, tokenID string, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, ts, token_id, token_label, src_ip, route, model, provider, via, tokens_in, tokens_out, latency_ms, status, cost_estimate, error FROM activity WHERE 1=1`
	args := []any{}
	if !since.IsZero() {
		q += ` AND ts >= ?`
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	if tokenID != "" {
		q += ` AND token_id = ?`
		args = append(args, tokenID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Activity
	for rows.Next() {
		var a Activity
		var ts string
		var cost sql.NullFloat64
		if err := rows.Scan(&a.ID, &ts, &a.TokenID, &a.TokenLabel, &a.SrcIP, &a.Route, &a.Model, &a.Provider, &a.Via,
			&a.TokensIn, &a.TokensOut, &a.LatencyMs, &a.Status, &cost, &a.Error); err != nil {
			return nil, err
		}
		a.TS, _ = time.Parse(time.RFC3339Nano, ts)
		if cost.Valid {
			v := cost.Float64
			a.CostEstimate = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Audit ---

type AuditEvent struct {
	ID      int64     `json:"id"`
	TS      time.Time `json:"ts"`
	Event   string    `json:"event"`
	Detail  string    `json:"detail"`
	SrcIP   string    `json:"src_ip"`
	TokenID string    `json:"token_id,omitempty"`
}

func (s *Store) InsertAudit(event, detail, srcIP, tokenID string) error {
	_, err := s.db.Exec(`INSERT INTO audit (ts, event, detail, src_ip, token_id) VALUES (?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano), event, detail, srcIP, tokenID)
	return err
}

func (s *Store) ListAudit(limit int) ([]AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, ts, event, detail, src_ip, token_id FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Event, &e.Detail, &e.SrcIP, &e.TokenID); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

// StatsRow is a simple rollup.
type StatsRow struct {
	Key       string  `json:"key"`
	Requests  int     `json:"requests"`
	TokensIn  int     `json:"tokens_in"`
	TokensOut int     `json:"tokens_out"`
	AvgLatMs  float64 `json:"avg_latency_ms"`
	Errors    int     `json:"errors"`
}

func (s *Store) StatsBy(group string, since time.Time) ([]StatsRow, error) {
	col := "route"
	switch group {
	case "model":
		col = "model"
	case "token":
		col = "token_label"
	case "day":
		// SQLite date
		col = "substr(ts, 1, 10)"
	case "route":
		col = "route"
	default:
		return nil, fmt.Errorf("unknown group %q", group)
	}
	q := fmt.Sprintf(`
SELECT %s AS k,
  COUNT(*) AS n,
  COALESCE(SUM(tokens_in),0),
  COALESCE(SUM(tokens_out),0),
  COALESCE(AVG(latency_ms),0),
  SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END)
FROM activity WHERE ts >= ?
GROUP BY k ORDER BY n DESC`, col)
	rows, err := s.db.Query(q, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StatsRow
	for rows.Next() {
		var r StatsRow
		if err := rows.Scan(&r.Key, &r.Requests, &r.TokensIn, &r.TokensOut, &r.AvgLatMs, &r.Errors); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
