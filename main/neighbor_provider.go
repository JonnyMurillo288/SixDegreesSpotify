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
		limit = 50
	}

	if verbose {
		fmt.Printf("\n=== Artist-Relation Neighbors for %s (%s) ===\n", a.Name, a.ID)
	}

	// ---------------------------------------------------------
	// 1. Lookup internal ID
	// ---------------------------------------------------------

	var internalID int
	if err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM artist WHERE gid = $1`, a.ID,
	).Scan(&internalID); err != nil {
		return nil, 500, fmt.Errorf("artist lookup failed: %w", err)
	}

	// ---------------------------------------------------------
	// 2. Get direct neighbor artists (l_artist_artist)
	// ---------------------------------------------------------

	q := `
        SELECT 
            a2.gid::text AS mbid,
            a2.name,
            lt.name AS link_type
        FROM l_artist_artist laa
        JOIN artist a1 ON laa.entity0 = a1.id
        JOIN artist a2 ON laa.entity1 = a2.id
        JOIN link l ON laa.link = l.id
        JOIN link_type lt ON l.link_type = lt.id
        WHERE a1.id = $1

        UNION ALL

        SELECT 
            a1.gid::text AS mbid,
            a1.name,
            lt.name AS link_type
        FROM l_artist_artist laa
        JOIN artist a1 ON laa.entity0 = a1.id
        JOIN artist a2 ON laa.entity1 = a2.id
        JOIN link l ON laa.link = l.id
        JOIN link_type lt ON l.link_type = lt.id
        WHERE a2.id = $1

        LIMIT $2;
    `

	rows, err := s.DB.QueryContext(ctx, q, internalID, limit)
	if err != nil {
		return nil, 500, fmt.Errorf("relationship lookup failed: %w", err)
	}
	defer rows.Close()

	results := []*NeighborEdge{}
	seen := map[string]bool{}

	// ---------------------------------------------------------
	// 3. For each neighbor, get the shared track
	// ---------------------------------------------------------
	for rows.Next() {
		var mbid, name, linkType string
		if err := rows.Scan(&mbid, &name, &linkType); err != nil {
			return nil, 500, err
		}
		if seen[mbid] {
			continue
		}
		seen[mbid] = true

		// Lookup shared track
		tr, err := s.getSharedTrack(ctx, a.ID, mbid)
		if err != nil {
			continue // skip if no shared track found
		}

		results = append(results, &NeighborEdge{
			Artist: &sixdegrees.Artists{
				ID:   mbid,
				Name: name,
			},
			Track: tr,
			Link:  linkType,
		})

		if len(results) >= limit {
			break
		}
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
        FROM recording r
        JOIN track t              ON t.recording = r.id
        JOIN medium m             ON m.id = t.medium
        JOIN release rls          ON rls.id = m.release
        JOIN artist_credit ac         ON ac.id = r.artist_credit
        JOIN artist_credit_name acn1  ON acn1.artist_credit = ac.id
        JOIN artist_credit_name acn2  ON acn2.artist_credit = ac.id
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
