package domain

import (
	"strings"
	"testing"
)

func sectionMatchSnapshot() Snapshot {
	return Snapshot{
		Programs:  []Program{{ID: 1, Active: 1}},
		Periods:   []Period{{ID: 1, Semester: "First", Active: 1}, {ID: 2, Semester: "2nd Semester", Active: 1}},
		Curricula: []Curriculum{{ID: 1, ProgramID: 1, YearLevel: 1, Semester: "1st Semester", Active: 1}, {ID: 2, ProgramID: 1, YearLevel: 1, Semester: "2nd Semester", Active: 1}, {ID: 3, ProgramID: 1, YearLevel: 0, Active: 1}},
		Mappings:  []Mapping{{CurriculumID: 3, Year: 1}},
	}
}
func TestResolveSectionCurriculum(t *testing.T) {
	s := sectionMatchSnapshot()
	for _, tt := range []struct{ period, want int }{{1, 1}, {2, 2}} {
		got, err := ResolveSectionCurriculum(s, 1, 1, tt.period)
		if err != nil || got != tt.want {
			t.Fatalf("period=%d got=%d error=%v", tt.period, got, err)
		}
	}
	s.Curricula = s.Curricula[2:]
	got, err := ResolveSectionCurriculum(s, 1, 1, 2)
	if err != nil || got != 3 {
		t.Fatalf("legacy fallback = %d, %v", got, err)
	}
	s.Mappings = nil
	if _, err = ResolveSectionCurriculum(s, 1, 1, 2); err == nil || !strings.Contains(err.Error(), "no active curriculum") {
		t.Fatalf("missing: %v", err)
	}
	s = sectionMatchSnapshot()
	s.Curricula = append(s.Curricula, Curriculum{ID: 4, ProgramID: 1, YearLevel: 1, Semester: "1st Semester", Active: 1})
	if _, err = ResolveSectionCurriculum(s, 1, 1, 1); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous: %v", err)
	}
}
