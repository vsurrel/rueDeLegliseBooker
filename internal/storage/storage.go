package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Reservation represents a stored reservation record.
type Reservation struct {
	ID      int64     `json:"id"`
	Person  string    `json:"person"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Comment string    `json:"comment"`
}

// Store provides persistence helpers backed by SQLite.
type Store struct {
	db *sql.DB
}

// New initialises the SQLite database and returns a Store.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=1&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	if err := initialise(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ListReservations returns every reservation ordered by start date.
func (s *Store) ListReservations(ctx context.Context) ([]Reservation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, person, start, end, comment FROM reservations ORDER BY start`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Reservation
	for rows.Next() {
		var (
			id      int64
			person  string
			start   string
			end     string
			comment sql.NullString
		)
		if err := rows.Scan(&id, &person, &start, &end, &comment); err != nil {
			return nil, err
		}

		startTime, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return nil, err
		}
		endTime, err := time.Parse(time.RFC3339, end)
		if err != nil {
			return nil, err
		}

		res = append(res, Reservation{
			ID:      id,
			Person:  person,
			Start:   startTime,
			End:     endTime,
			Comment: comment.String,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

// CreateReservation persists a reservation and returns its identifier.
func (s *Store) CreateReservation(ctx context.Context, r Reservation) (int64, error) {
	if r.Person == "" {
		return 0, errors.New("person is required")
	}
	if !r.End.After(r.Start) {
		return 0, errors.New("end must be after start")
	}

	res, err := s.db.ExecContext(
		ctx,
		`INSERT INTO reservations (person, start, end, comment) VALUES (?, ?, ?, ?)`,
		r.Person,
		r.Start.Format(time.RFC3339),
		r.End.Format(time.RFC3339),
		r.Comment,
	)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// DeleteReservation removes the reservation matching the provided ID.
func (s *Store) DeleteReservation(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM reservations WHERE id = ?`, id)
	return err
}

// UpdateReservationComment updates the comment attached to the reservation.
func (s *Store) UpdateReservationComment(ctx context.Context, id int64, comment string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reservations SET comment = ? WHERE id = ?`, comment, id)
	return err
}

func initialise(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS reservations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		person TEXT NOT NULL,
		start TEXT NOT NULL,
		end TEXT NOT NULL,
		comment TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_reservations_range ON reservations(start, end);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	return ensureCommentColumn(db)
}

func ensureCommentColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(reservations)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notnull    int
			dfltValue  any
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &primaryKey); err != nil {
			return err
		}
		if name == "comment" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE reservations ADD COLUMN comment TEXT`)
	return err
}
