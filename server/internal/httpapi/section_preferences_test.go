package httpapi

import (
	"encoding/json"
	"testing"
)

func TestDeveloperBatchSectionsReuseCodesAcrossYears(t *testing.T) {
	h := newHarness(t)
	h.fixture()

	_, reply := h.call("POST", "/api/developer/sections/batch", map[string]any{
		"program_id": 1, "year_level": 1, "codes": []string{"A", "B", "C"}, "size": 35,
	}, nil, 200)
	var created struct {
		Added int `json:"added"`
	}
	if err := json.Unmarshal(reply, &created); err != nil || created.Added != 3 {
		t.Fatalf("expected 3 batch sections, got %s: %v", reply, err)
	}
	// A section code may be shared by a different year in the SAME program.
	h.call("POST", "/api/developer/sections/batch", map[string]any{
		"program_id": 1, "year_level": 2, "codes": []string{"A", "B"}, "size": 40,
	}, nil, 200)
	// A code may also be reused by another program in the same year.
	h.call("POST", "/api/developer/sections/batch", map[string]any{
		"program_id": 2, "year_level": 1, "codes": []string{"A"}, "size": 40,
	}, nil, 200)
	// But duplicates in the same program/year are never created.
	h.call("POST", "/api/developer/sections/batch", map[string]any{
		"program_id": 1, "year_level": 1, "codes": []string{"A", "D"}, "size": 40,
	}, nil, 409)
	if got := h.count("sections"); got != 8 { // two original plus 3+2+1
		t.Fatalf("failed batch was not atomic; count=%d", got)
	}
	var advisers int
	if err := h.store.DB.QueryRow("SELECT count(*) FROM sections WHERE code IN ('A','B','C') AND adviser_id IS NOT NULL").Scan(&advisers); err != nil || advisers != 0 {
		t.Fatalf("batch advisers must be unassigned: %d (%v)", advisers, err)
	}
}

func TestFacultyPreferencesValidateAndLoad(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	request := map[string]any{
		"first_start_minute": 540, "first_end_minute": 630,
		"second_start_minute": 780, "second_end_minute": 870,
		"preferred_room_id": 1,
	}
	h.call("POST", "/api/faculty/1/preferences", request, nil, 200)
	_, response := h.call("GET", "/api/bootstrap", nil, nil, 200)
	var snap struct {
		Data struct {
			Preferences []struct {
				FacultyID       int `json:"faculty_id"`
				FirstStart      int `json:"first_start_minute"`
				PreferredRoomID int `json:"preferred_room_id"`
			} `json:"faculty_preferences"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response, &snap); err != nil {
		t.Fatalf("bootstrap not readable: %v", err)
	}
	if len(snap.Data.Preferences) != 1 || snap.Data.Preferences[0].FacultyID != 1 || snap.Data.Preferences[0].FirstStart != 0 || snap.Data.Preferences[0].PreferredRoomID != 1 {
		t.Fatalf("bootstrap failed to include faculty preferences: %s", response)
	}
	h.call("POST", "/api/faculty/1/preferences", map[string]any{
		"first_start_minute": 0, "first_end_minute": 0,
		"second_start_minute": 780, "second_end_minute": 870,
	}, nil, 200)
	h.call("POST", "/api/faculty/1/preferences", map[string]any{
		"first_start_minute": 545, "first_end_minute": 630,
	}, nil, 200)
	// Retired time fields are ignored. Facility remains optional and can be cleared.
	h.call("POST", "/api/faculty/1/preferences", map[string]any{}, nil, 200)
	var start, room int
	if err := h.store.DB.QueryRow("SELECT first_start_minute,COALESCE(preferred_room_id,0) FROM faculty_preferences WHERE faculty_id=1").Scan(&start, &room); err != nil || start != 0 || room != 0 {
		t.Fatalf("optional preferences not cleared: %d %d (%v)", start, room, err)
	}
}
