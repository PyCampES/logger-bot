// Package store wraps a SQLite-backed workout log.
//
// Two roles, two connections (mirrors the original Python design):
//   - Writer:  read-write, owns the schema migration on first use.
//   - Reader:  ?mode=ro DSN, used by /last and /sql. Read-only is the
//              safety boundary that makes raw user-supplied SQL acceptable.
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/santiago-jauregui/logger-bot/internal/extraction"
	_ "modernc.org/sqlite"
)

const tableName = "workout"

const createTableSQL = `
CREATE TABLE IF NOT EXISTS workout (
    date TEXT,
    time TEXT,
    category TEXT,
    exercise TEXT,
    reps TEXT,
    weight TEXT,
    unit TEXT,
    raw_text TEXT
)`

// Row mirrors the workout schema. Fields are strings because the original
// schema stores everything as TEXT.
type Row struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	Category string `json:"category"`
	Exercise string `json:"exercise"`
	Reps     string `json:"reps"`
	Weight   string `json:"weight"`
	Unit     string `json:"unit"`
	RawText  string `json:"raw_text"`
}

// Writer handles INSERTs.
type Writer struct{ db *sql.DB }

func NewWriter(path string) (*Writer, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(createTableSQL); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Writer{db: db}, nil
}

func (w *Writer) Write(e extraction.Workout) error {
	now := time.Now()
	_, err := w.db.Exec(
		`INSERT INTO workout (date, time, category, exercise, reps, weight, unit, raw_text)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		now.Format("2006-01-02"),
		now.Format("15:04:05"),
		e.Category,
		e.Exercise,
		fmt.Sprintf("%d", e.Reps),
		fmt.Sprintf("%g", e.Weight),
		e.Unit,
		e.RawText,
	)
	return err
}

func (w *Writer) Close() error { return w.db.Close() }

// Reader executes SELECTs against a read-only DSN.
type Reader struct{ db *sql.DB }

func NewReader(path string) (*Reader, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	return &Reader{db: db}, nil
}

func (r *Reader) Close() error { return r.db.Close() }

// Last returns the most recent row for an exercise (case-insensitive substring),
// or nil if none. Mirrors the Python `/last` query.
func (r *Reader) Last(exercise string) (*Row, error) {
	const q = `SELECT date, time, category, exercise, reps, weight, unit, raw_text
	           FROM workout
	           WHERE exercise LIKE ? COLLATE NOCASE
	           ORDER BY rowid DESC
	           LIMIT 1`
	row := r.db.QueryRow(q, "%"+exercise+"%")

	var x Row
	err := row.Scan(&x.Date, &x.Time, &x.Category, &x.Exercise, &x.Reps, &x.Weight, &x.Unit, &x.RawText)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &x, nil
}

// Query runs an arbitrary SQL string. Used by /sql. The read-only DSN is the
// safety boundary — DO NOT add a write connection here.
func (r *Reader) Query(query string) ([]map[string]any, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}