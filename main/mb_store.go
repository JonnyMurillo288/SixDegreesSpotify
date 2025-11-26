package main

import (
	"context"
	"database/sql"
	"os"
	"time"

	_ "github.com/lib/pq" // Postgres driver
)

//
// ========================================================================
// Store: Postgres-backed MusicBrainz Database Wrapper
// ========================================================================
//

type Store struct {
	DB *sql.DB
}

// Open initializes a PostgreSQL-backed Store.
// If DSN is empty, it uses PG_DSN env or a default local MusicBrainz DSN.
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		dsn = os.Getenv("PG_DSN")
		if dsn == "" {
			dsn = "postgres://postgres:password@localhost:5432/musicbrainz?sslmode=disable"
		}
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	// Test connectivity
	err = withTimeout(func(ctx context.Context) error {
		return db.PingContext(ctx)
	}, 5*time.Second)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{DB: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

//
// ========================================================================
// Core Data Structures
// ========================================================================
//

// DBArtistCredit represents a credited artist on a recording/track.
type DBArtistCredit struct {
	ID   string
	Name string
}

// DBTrack represents a single track + recording + cover art + collaborator list.
type DBTrack struct {
	RecordingMBID string
	RecordingName string
	TrackMBID     string
	TrackName     string
	CoverURL      string
	ArtistCredits []DBArtistCredit
}

// MBRecordingSearchDB is the wrapper returned by GetArtistTracks.
type MBRecordingSearchDB struct {
	Recordings []DBTrack
}

//
// ========================================================================
// GetArtistTracks: Full Track + Recording + Cover Art + Collaborators
// ========================================================================
//

// GetArtistTracks returns all tracks & collaborators for a given artist MBID.
func (s *Store) GetArtistTracks(
	ctx context.Context,
	mbid string,
	limit int,
) (MBRecordingSearchDB, error) {

	var result MBRecordingSearchDB

	q := `
WITH target AS (
    SELECT id FROM artist WHERE gid = $1
),
tracks AS (
    SELECT
        r.id                      AS recording_id,
        r.gid::text              AS recording_mbid,
        r.name                   AS recording_name,
        t.gid::text              AS track_mbid,
        t.name                   AS track_name,
        rls.gid::text            AS release_mbid
    FROM recording r
    JOIN track t                 ON t.recording = r.id
    JOIN medium m                ON m.id = t.medium
    JOIN release rls             ON rls.id = m.release
    JOIN artist_credit ac        ON ac.id = r.artist_credit
    JOIN artist_credit_name acn  ON acn.artist_credit = ac.id
    JOIN target ta               ON acn.artist = ta.id
    LIMIT $2
)
SELECT
    recording_mbid,
    recording_name,
    track_mbid,
    track_name,
    'https://coverartarchive.org/release/' || release_mbid || '/front' AS cover_url
FROM tracks;
`

	rows, err := s.DB.QueryContext(ctx, q, mbid, limit)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var t DBTrack

		if err := rows.Scan(
			&t.RecordingMBID,
			&t.RecordingName,
			&t.TrackMBID,
			&t.TrackName,
			&t.CoverURL,
		); err != nil {
			return result, err
		}

		// Load all credited artists on the recording.
		credits, err := s.getRecordingCredits(ctx, t.RecordingMBID)
		if err != nil {
			return result, err
		}

		t.ArtistCredits = credits
		result.Recordings = append(result.Recordings, t)
	}

	return result, nil
}

//
// ========================================================================
// getRecordingCredits: small helper to fetch credited artists
// ========================================================================
//

func (s *Store) getRecordingCredits(ctx context.Context, recordingMBID string) ([]DBArtistCredit, error) {
	q := `
SELECT a.gid::text, a.name
FROM recording r
JOIN artist_credit ac        ON ac.id = r.artist_credit
JOIN artist_credit_name acn  ON acn.artist_credit = ac.id
JOIN artist a               ON a.id = acn.artist
WHERE r.gid = $1;
`
	rows, err := s.DB.QueryContext(ctx, q, recordingMBID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var credits []DBArtistCredit

	for rows.Next() {
		var ac DBArtistCredit
		if err := rows.Scan(&ac.ID, &ac.Name); err != nil {
			return nil, err
		}
		credits = append(credits, ac)
	}

	return credits, nil
}

//
// ========================================================================
// ArtistInternal: internal numeric ID lookup
// ========================================================================
//

type ArtistInternal struct {
	ID   int    // numeric MusicBrainz artist.id
	MBID string // UUID (gid)
	Name string
}

func (s *Store) LookupArtistByMBID(mbid string) (*ArtistInternal, error) {
	q := `
SELECT id, gid::text, name
FROM artist
WHERE gid = $1
LIMIT 1;
`
	var a ArtistInternal
	err := s.DB.QueryRow(q, mbid).Scan(&a.ID, &a.MBID, &a.Name)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

//
// ========================================================================
// Utilities
// ========================================================================
//

func withTimeout(fn func(ctx context.Context) error, d time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return fn(ctx)
}
