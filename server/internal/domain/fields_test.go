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
