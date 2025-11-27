package main

import (
	"context"
	"fmt"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

//
// ----------------------------------------------------------
// Shared Structs
// ----------------------------------------------------------
//

// Returned by all neighbor providers.
type NeighborEdge struct {
	Artist *sixdegrees.Artists
	Track  sixdegrees.Track
	Link   string // relationship type
}

//
// ----------------------------------------------------------
// API-based MusicBrainz Recording Search
// ----------------------------------------------------------
//

type MBRecordingSearch struct {
	Recordings []struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		ArtistCredit []struct {
			Artist struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"artist-credit"`
	} `json:"recordings"`
}

func (s *Store) MusicBrainzNeighborProvider(
	ctx context.Context,
	a *sixdegrees.Artists,
	limit int,
	verbose bool,
) ([]*NeighborEdge, int, error) {

	if a == nil || a.ID == "" {
		return nil, 400, fmt.Errorf("artist missing MBID")
	}
	if limit <= 0 {
		limit = 200
	}

	if verbose {
		fmt.Printf("\n=== Track-Based Neighbors for %s (%s) ===\n", a.Name, a.ID)
	}

	// ---------------------------------------------------------
	// 1. Lookup internal MB artist ID
	// ---------------------------------------------------------
	var internalID int
	err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM artist WHERE gid = $1`, a.ID,
	).Scan(&internalID)
	if err != nil {
		return nil, 500, fmt.Errorf("artist lookup failed: %w", err)
	}

	// ---------------------------------------------------------
	// 2. Fetch all artists who appear on ANY track with A
	// ---------------------------------------------------------
	q := `
        WITH target AS (SELECT id FROM artist WHERE gid = $1)

        SELECT DISTINCT
            a2.gid::text AS mbid,
            a2.name,
            r.gid::text AS recording_mbid,
            r.name      AS recording_name,
            t.gid::text AS track_mbid,
            t.name      AS track_name,
            rls.gid::text AS release_mbid
        FROM track t
        JOIN recording r              ON r.id = t.recording
        JOIN medium m                 ON m.id = t.medium
        JOIN release rls              ON rls.id = m.release
        JOIN artist_credit ac         ON ac.id = t.artist_credit
        JOIN artist_credit_name acn1  ON acn1.artist_credit = ac.id
        JOIN artist_credit_name acn2  ON acn2.artist_credit = ac.id
        JOIN target ta                ON acn1.artist = ta.id
        JOIN artist a2                ON a2.id = acn2.artist
        WHERE acn2.artist != ta.id
        LIMIT $2;
    `

	rows, err := s.DB.QueryContext(ctx, q, a.ID, limit)
	if err != nil {
		return nil, 500, fmt.Errorf("track-collab lookup failed: %w", err)
	}
	defer rows.Close()

	results := []*NeighborEdge{}

	for rows.Next() {
		var mbid, nbName string
		var recMBID, recName, trackMBID, trackName, releaseMBID string

		err := rows.Scan(
			&mbid, &nbName,
			&recMBID, &recName,
			&trackMBID, &trackName,
			&releaseMBID,
		)
		if err != nil {
			return nil, 500, err
		}

		results = append(results, &NeighborEdge{
			Artist: &sixdegrees.Artists{
				ID:   mbid,
				Name: nbName,
			},
			Track: sixdegrees.Track{
				ID:        trackMBID,
				Name:      trackName,
				Recording: recMBID,
				PhotoURL:  "https://coverartarchive.org/release/" + releaseMBID + "/front",
			},
			Link: "track-collaboration",
		})
	}

	if verbose {
		fmt.Printf("Found %d track-based neighbors.\n", len(results))
	}

	return results, 200, nil
}

func (s *Store) getSharedTrack(ctx context.Context, aMBID, bMBID string) (sixdegrees.Track, error) {

	q := `
        WITH a1 AS (SELECT id FROM artist WHERE gid = $1),
             a2 AS (SELECT id FROM artist WHERE gid = $2)
        SELECT DISTINCT
            r.gid::text  AS recording_mbid,
            r.name       AS recording_name,
            t.gid::text  AS track_mbid,
            t.name       AS track_name,
            rls.gid::text AS release_mbid
        FROM track t
        JOIN recording r           ON r.id = t.recording
        JOIN medium m              ON m.id = t.medium
        JOIN release rls           ON rls.id = m.release
        JOIN artist_credit ac      ON ac.id = t.artist_credit
        JOIN artist_credit_name acn1 ON acn1.artist_credit = ac.id
        JOIN artist_credit_name acn2 ON acn2.artist_credit = ac.id
        JOIN a1 ON acn1.artist = a1.id
        JOIN a2 ON acn2.artist = a2.id
        LIMIT 1;
    `

	var recMBID, recName, trackMBID, trackName, releaseMBID string

	err := s.DB.QueryRowContext(ctx, q, aMBID, bMBID).Scan(
		&recMBID, &recName, &trackMBID, &trackName, &releaseMBID,
	)
	if err != nil {
		return sixdegrees.Track{}, err
	}

	return sixdegrees.Track{
		ID:        trackMBID,
		Name:      trackName,
		Recording: recMBID,
		PhotoURL:  "https://coverartarchive.org/release/" + releaseMBID + "/front",
	}, nil
}

//
// ----------------------------------------------------------
// DB-based Track Search (Recording + Track + Artist Credit + Cover Art)
// ----------------------------------------------------------
//

func (s *Store) GetArtistTracksDB(
	ctx context.Context,
	mbid string,
	limit int,
) (MBRecordingSearchDB, error) {

	var result MBRecordingSearchDB

	q := `
		WITH target_artist AS (
			SELECT id FROM artist WHERE gid = $1
		),
		tracks AS (
			SELECT
				r.id AS recording_id,
				r.gid::text AS recording_mbid,
				r.name AS recording_name,
				t.gid::text AS track_mbid,
				t.name AS track_name,
				rls.gid::text AS release_mbid
			FROM recording r
			JOIN track t        ON t.recording = r.id
			JOIN medium m       ON m.id = t.medium
			JOIN release rls    ON rls.id = m.release
			JOIN artist_credit ac  ON ac.id = r.artist_credit
			JOIN artist_credit_name acn ON acn.artist_credit = ac.id
			JOIN target_artist ta ON acn.artist = ta.id
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
		var r DBTrack

		if err := rows.Scan(
			&r.RecordingMBID,
			&r.RecordingName,
			&r.TrackMBID,
			&r.TrackName,
			&r.CoverURL,
		); err != nil {
			return result, err
		}

		// Load artist credits for each recording
		acq := `
			SELECT a.gid::text, a.name
			FROM recording r
			JOIN artist_credit ac ON ac.id = r.artist_credit
			JOIN artist_credit_name acn ON acn.artist_credit = ac.id
			JOIN artist a ON a.id = acn.artist
			WHERE r.gid = $1;
			`
		acRows, err := s.DB.QueryContext(ctx, acq, r.RecordingMBID)
		if err != nil {
			return result, err
		}

		var credits []DBArtistCredit

		for acRows.Next() {
			var ac DBArtistCredit
			if err := acRows.Scan(&ac.ID, &ac.Name); err != nil {
				acRows.Close()
				return result, err
			}
			credits = append(credits, ac)
		}
		acRows.Close()

		r.ArtistCredits = credits
		result.Recordings = append(result.Recordings, r)
	}

	return result, nil
}
