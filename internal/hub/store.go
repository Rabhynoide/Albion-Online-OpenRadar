// Package hub implements the OpenRadar Hub: a small self-hostable service that lets a
// group of radar clients pool discovered Avalonian Road edges into one shared database,
// instead of each player only ever seeing their own local discoveries.
package hub

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nospy/albion-openradar/internal/roads"
)

// Store is a SQLite-backed collection of discovered road edges, shared across every
// client submitting to this Hub instance.
type Store struct {
	db *sql.DB
}

// OpenStore opens (creating if necessary) the SQLite database at path and ensures its
// schema exists.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // modernc.org/sqlite: avoid concurrent-writer lock contention
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS edges (
			"from" TEXT NOT NULL,
			"to" TEXT NOT NULL,
			pos_x REAL,
			pos_y REAL,
			discovered_at TIMESTAMP NOT NULL,
			PRIMARY KEY ("from", "to")
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// UpsertEdge inserts or refreshes a (from,to) edge, mirroring roads.AddEdge's
// upsert-refreshes-timestamp semantics.
func (s *Store) UpsertEdge(from, to string, pos *[2]float64) error {
	var posX, posY sql.NullFloat64
	if pos != nil {
		posX = sql.NullFloat64{Float64: pos[0], Valid: true}
		posY = sql.NullFloat64{Float64: pos[1], Valid: true}
	}
	_, err := s.db.Exec(`
		INSERT INTO edges ("from", "to", pos_x, pos_y, discovered_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT ("from", "to") DO UPDATE SET
			pos_x = excluded.pos_x,
			pos_y = excluded.pos_y,
			discovered_at = excluded.discovered_at
	`, from, to, posX, posY, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("upsert edge: %w", err)
	}
	return nil
}

// ListEdges returns every known edge.
func (s *Store) ListEdges() ([]roads.Edge, error) {
	rows, err := s.db.Query(`SELECT "from", "to", pos_x, pos_y, discovered_at FROM edges`)
	if err != nil {
		return nil, fmt.Errorf("list edges: %w", err)
	}
	defer rows.Close()

	edges := make([]roads.Edge, 0)
	for rows.Next() {
		var e roads.Edge
		var posX, posY sql.NullFloat64
		if err := rows.Scan(&e.From, &e.To, &posX, &posY, &e.DiscoveredAt); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		if posX.Valid && posY.Valid {
			e.Pos = &[2]float64{posX.Float64, posY.Float64}
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edges: %w", err)
	}
	return edges, nil
}
