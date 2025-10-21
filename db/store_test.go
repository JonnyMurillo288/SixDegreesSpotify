package db

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
)

func TestDBArtist_GenresJSONRoundTrip(t *testing.T) {
	orig := map[string]int{
		"rock": 2,
		"pop":  1,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("failed to marshal genres: %v", err)
	}

	var got map[string]int
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal genres: %v", err)
	}

	if !reflect.DeepEqual(orig, got) {
		t.Fatalf("genres round-trip mismatch. want=%v got=%v", orig, got)
	}
}

func TestSQLNullFields_WithValidTrueRetainValues(t *testing.T) {
	album := DBAlbum{
		ID:              "al1",
		Name:            sql.NullString{String: "Album Name", Valid: true},
		PrimaryArtistID: sql.NullString{String: "ar1", Valid: true},
	}
	artist := DBArtist{
		ID:         "ar1",
		Name:       "Artist Name",
		Popularity: sql.NullInt64{Int64: 55, Valid: true},
		Genres:     map[string]int{"indie": 3},
	}
	track := DBTrack{
		ID:              "t1",
		Name:            "Track Name",
		AlbumID:         sql.NullString{String: "al1", Valid: true},
		PrimaryArtistID: sql.NullString{String: "ar1", Valid: true},
	}

	if !album.Name.Valid || album.Name.String != "Album Name" {
		t.Fatalf("album.Name not retained. got Valid=%v, String=%q", album.Name.Valid, album.Name.String)
	}
	if !album.PrimaryArtistID.Valid || album.PrimaryArtistID.String != "ar1" {
		t.Fatalf("album.PrimaryArtistID not retained. got Valid=%v, String=%q", album.PrimaryArtistID.Valid, album.PrimaryArtistID.String)
	}
	if !artist.Popularity.Valid || artist.Popularity.Int64 != 55 {
		t.Fatalf("artist.Popularity not retained. got Valid=%v, Int64=%d", artist.Popularity.Valid, artist.Popularity.Int64)
	}
	if !track.AlbumID.Valid || track.AlbumID.String != "al1" {
		t.Fatalf("track.AlbumID not retained. got Valid=%v, String=%q", track.AlbumID.Valid, track.AlbumID.String)
	}
	if !track.PrimaryArtistID.Valid || track.PrimaryArtistID.String != "ar1" {
		t.Fatalf("track.PrimaryArtistID not retained. got Valid=%v, String=%q", track.PrimaryArtistID.Valid, track.PrimaryArtistID.String)
	}
}

func TestTrackAndAlbum_AssignAndReadForeignKeys(t *testing.T) {
	album := DBAlbum{
		ID:              "album-123",
		Name:            sql.NullString{String: "Some Album", Valid: true},
		PrimaryArtistID: sql.NullString{String: "artist-999", Valid: true},
	}
	track := DBTrack{
		ID:              "track-abc",
		Name:            "Some Track",
		AlbumID:         sql.NullString{String: "album-123", Valid: true},
		PrimaryArtistID: sql.NullString{String: "artist-999", Valid: true},
	}

	if !album.PrimaryArtistID.Valid || album.PrimaryArtistID.String != "artist-999" {
		t.Fatalf("album.PrimaryArtistID mismatch. got=%v", album.PrimaryArtistID)
	}
	if !track.AlbumID.Valid || track.AlbumID.String != "album-123" {
		t.Fatalf("track.AlbumID mismatch. got=%v", track.AlbumID)
	}
	if !track.PrimaryArtistID.Valid || track.PrimaryArtistID.String != "artist-999" {
		t.Fatalf("track.PrimaryArtistID mismatch. got=%v", track.PrimaryArtistID)
	}
}

func TestDBArtist_GenresNilVsEmpty(t *testing.T) {
	var nilMap map[string]int // nil
	emptyMap := map[string]int{}

	nilJSON, err := json.Marshal(nilMap)
	if err != nil {
		t.Fatalf("marshal nil map error: %v", err)
	}
	emptyJSON, err := json.Marshal(emptyMap)
	if err != nil {
		t.Fatalf("marshal empty map error: %v", err)
	}

	if string(nilJSON) != "null" {
		t.Fatalf("expected nil map to marshal to 'null', got %q", string(nilJSON))
	}
	if string(emptyJSON) != "{}" {
		t.Fatalf("expected empty map to marshal to '{}', got %q", string(emptyJSON))
	}

	var outNil map[string]int
	if err := json.Unmarshal(nilJSON, &outNil); err != nil {
		t.Fatalf("unmarshal nil json error: %v", err)
	}
	var outEmpty map[string]int
	if err := json.Unmarshal(emptyJSON, &outEmpty); err != nil {
		t.Fatalf("unmarshal empty json error: %v", err)
	}

	if outNil != nil {
		t.Fatalf("expected unmarshaled nil JSON to result in nil map, got non-nil")
	}
	if outEmpty == nil || len(outEmpty) != 0 {
		t.Fatalf("expected unmarshaled '{}' to result in non-nil empty map, got %#v", outEmpty)
	}
	if reflect.DeepEqual(outNil, outEmpty) {
		t.Fatalf("nil and empty maps should be distinguishable after round-trip")
	}
}

func TestSQLNullFields_WithValidFalseRemainNull(t *testing.T) {
	album := DBAlbum{
		ID:              "al2",
		Name:            sql.NullString{String: "", Valid: false},
		PrimaryArtistID: sql.NullString{String: "", Valid: false},
	}
	artist := DBArtist{
		ID:         "ar2",
		Name:       "Artist 2",
		Popularity: sql.NullInt64{Int64: 0, Valid: false},
	}

	if album.Name.Valid || album.Name.String != "" {
		t.Fatalf("album.Name should be NULL. got Valid=%v, String=%q", album.Name.Valid, album.Name.String)
	}
	if album.PrimaryArtistID.Valid || album.PrimaryArtistID.String != "" {
		t.Fatalf("album.PrimaryArtistID should be NULL. got Valid=%v, String=%q", album.PrimaryArtistID.Valid, album.PrimaryArtistID.String)
	}
	if artist.Popularity.Valid || artist.Popularity.Int64 != 0 {
		t.Fatalf("artist.Popularity should be NULL. got Valid=%v, Int64=%d", artist.Popularity.Valid, artist.Popularity.Int64)
	}
}

func TestPopularity_ZeroValueVsNull(t *testing.T) {
	withZero := DBArtist{
		ID:         "ar3",
		Name:       "Artist 3",
		Popularity: sql.NullInt64{Int64: 0, Valid: true},
	}
	withNull := DBArtist{
		ID:         "ar4",
		Name:       "Artist 4",
		Popularity: sql.NullInt64{Int64: 0, Valid: false},
	}

	if !withZero.Popularity.Valid || withZero.Popularity.Int64 != 0 {
		t.Fatalf("expected Popularity=0 with Valid=true to be preserved, got Valid=%v Int64=%d", withZero.Popularity.Valid, withZero.Popularity.Int64)
	}
	if withNull.Popularity.Valid {
		t.Fatalf("expected Popularity to be NULL when Valid=false")
	}
	if reflect.DeepEqual(withZero.Popularity, withNull.Popularity) {
		t.Fatalf("Popularity=0 (Valid=true) should not be conflated with NULL (Valid=false)")
	}
}
