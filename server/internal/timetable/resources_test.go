package timetable

import "testing"

func TestResourceSheetsKeepReusedSectionCodesSeparate(t *testing.T) {
	entries := []Entry{
		{SectionID: 1, Program: "BSHM", YearLevel: 1, Section: "A", SubjectCode: "HM101", Day: 1},
		{SectionID: 2, Program: "BSHM", YearLevel: 2, Section: "A", SubjectCode: "HM201", Day: 2},
		{SectionID: 3, Program: "BSIT", YearLevel: 1, Section: "A", SubjectCode: "IT101", Day: 1},
		{SectionID: 1, Program: "BSHM", YearLevel: 1, Section: "A", SubjectCode: "HM101", Day: 3},
	}
	sheets := resourceSheets("sections", entries, "All")
	if len(sheets) != 3 {
		t.Fatalf("got %d resource sheets; want 3", len(sheets))
	}
	seen := map[string]int{}
	for _, sheet := range sheets {
		seen[sheet.Label] = len(sheet.Entries)
	}
	for label, count := range map[string]int{"BSHM / 1st year / A": 2, "BSHM / 2nd year / A": 1, "BSIT / 1st year / A": 1} {
		if seen[label] != count {
			t.Errorf("%s: %d meetings; want %d", label, seen[label], count)
		}
	}
}

func TestResourcesUseStableIDsAndOrdinalYears(t *testing.T) {
	entries := []Entry{{FacultyID: 1, Faculty: "Same name"}, {FacultyID: 2, Faculty: "Same name"}}
	if sheets := resourceSheets("faculty", entries, "All"); len(sheets) != 2 {
		t.Fatalf("same names merged: %d sheets", len(sheets))
	}
	for n, want := range map[int]string{1: "1st year", 2: "2nd year", 3: "3rd year", 4: "4th year", 11: "11th year", 12: "12th year", 13: "13th year", 21: "21st year"} {
		if got := yearLabel(n); got != want {
			t.Errorf("%d: %s; want %s", n, got, want)
		}
	}
	if sheets := resourceSheets("rooms", nil, "No room selected"); len(sheets) != 1 || sheets[0].Label != "No room selected" {
		t.Fatal("empty sheet fallback lost")
	}
}
