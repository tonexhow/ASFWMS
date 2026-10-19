package domain

import "testing"

func TestNormalizeRejectsInvalidAndUnknownFields(t *testing.T) {
	cases := []struct {
		k string
		v map[string]any
	}{{"subjects", map[string]any{"code": "A", "title": "A", "weekly_minutes": 61}}, {"departments", map[string]any{"code": "", "name": "Name"}}, {"faculty", map[string]any{"name": "Name", "admin": true}}, {"sections", map[string]any{"program_id": 1, "curriculum_id": 1, "period_id": 1, "code": "A", "year_level": 0}}, {"academic_periods", map[string]any{"label": "Test", "academic_year": "2026", "semester": "First", "start_date": "2026-13-40", "end_date": "2026-12-01"}}}
	for _, c := range cases {
		if _, e := Normalize(c.k, c.v, nil); e == nil {
			t.Fatalf("invalid %s accepted: %v", c.k, c.v)
		}
	}
}
func TestNormalizeDefaultsAndIndependentAdviser(t *testing.T) {
	out, e := Normalize("faculty", map[string]any{"name": " Faculty "}, nil)
	if e != nil || Str(out, "name") != "Faculty" || Int(out, "active") != 1 || Int(out, "max_weekly_minutes") != 0 {
		t.Fatal(out, e)
	}
	out, e = Normalize("subjects", map[string]any{"code": "S", "title": "Subject"}, nil)
	if e != nil || Int(out, "weekly_minutes") != 180 {
		t.Fatal(out, e)
	}
}

func TestFacultyIDNumberOptionalAndPreservesLegacyName(t *testing.T) {
	created, err := Normalize("faculty", map[string]any{"first_name": "Mae", "last_name": "Santos", "faculty_id_number": "  BCC-0007  "}, nil)
	if err != nil || Str(created, "name") != "Mae Santos" || Str(created, "faculty_id_number") != "BCC-0007" {
		t.Fatalf("invalid faculty ID create: %v / %v", created, err)
	}
	legacy := map[string]any{"name": "Dr. Existing Unsplit Teacher", "faculty_id_number": "", "employment": "Full Time"}
	modified, err := Normalize("faculty", map[string]any{"faculty_id_number": "000812"}, legacy)
	if err != nil || Str(modified, "name") != "Dr. Existing Unsplit Teacher" || Str(modified, "faculty_id_number") != "000812" {
		t.Fatalf("legacy name changed on ID edit: %v / %v", modified, err)
	}
	noID, err := Normalize("faculty", map[string]any{"name": "Legacy row"}, nil)
	if err != nil || Str(noID, "faculty_id_number") != "" {
		t.Fatalf("optional field required: %v / %v", noID, err)
	}
	if _, err = Normalize("faculty", map[string]any{"name": "Too long", "faculty_id_number": string(make([]byte, 51))}, nil); err == nil {
		t.Fatal("faculty ID length was not checked")
	}
	if _, err = Normalize("faculty", map[string]any{"name": "Bad", "faculty_id_number": "alpha\nbeta"}, nil); err == nil {
		t.Fatal("control characters accepted in identifier")
	}
}
