package store

import (
	"path/filepath"
	"testing"

	"github.com/santiago-jauregui/logger-bot/internal/extraction"
)

func TestWriteThenReadLast(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")

	w, err := NewWriter(dbPath)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	for _, e := range []extraction.Workout{
		{Category: "Pecho", Exercise: "Press De Banca", Reps: 10, Weight: 80, Unit: "kg", RawText: "press 80kg 10 reps"},
		{Category: "Pecho", Exercise: "Press De Banca", Reps: 8, Weight: 85, Unit: "kg", RawText: "press 85kg 8 reps"},
	} {
		if err := w.Write(e); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	r, err := NewReader(dbPath)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	got, err := r.Last("press de banca")
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if got == nil {
		t.Fatal("expected a row, got nil")
	}
	if got.Reps != "8" || got.Weight != "85" {
		t.Fatalf("expected most recent row (8 reps @ 85kg), got %+v", got)
	}
}

func TestLastReturnsNilWhenNoMatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")
	w, _ := NewWriter(dbPath)
	t.Cleanup(func() { _ = w.Close() })

	r, _ := NewReader(dbPath)
	t.Cleanup(func() { _ = r.Close() })

	got, err := r.Last("nonexistent")
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestReaderRejectsWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")
	w, _ := NewWriter(dbPath)
	t.Cleanup(func() { _ = w.Close() })

	r, _ := NewReader(dbPath)
	t.Cleanup(func() { _ = r.Close() })

	rows, err := r.Query("INSERT INTO workout (date) VALUES ('today')")
	if err == nil {
		t.Fatalf("expected error from write on read-only conn, got rows=%v", rows)
	}
}

func TestQueryRoundTripsJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log.db")
	w, _ := NewWriter(dbPath)
	t.Cleanup(func() { _ = w.Close() })
	_ = w.Write(extraction.Workout{Exercise: "Sentadilla", Reps: 5, Weight: 100, Unit: "kg"})

	r, _ := NewReader(dbPath)
	t.Cleanup(func() { _ = r.Close() })

	rows, err := r.Query("SELECT exercise, reps FROM workout")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["exercise"] != "Sentadilla" {
		t.Fatalf("got %+v", rows[0])
	}
}