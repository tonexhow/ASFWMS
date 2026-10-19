// Package timetable renders portable, escaped HTML for faculty and room sheets.
// Grouping is by actual meetings, never by an assumed MWF/TTh label.
package timetable

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Entry struct {
	SectionID                                                                    int64
	OfferingID, FacultyID, RoomID                                                int64
	Faculty, Room, Department, Program, Section, SubjectCode, SubjectTitle, Kind string
	YearLevel, Day, Start, End                                                   int
	Units                                                                        float64
}
type Frame struct {
	Term, AcademicYear, Semester, Description, PrintedAt string
	Version                                              int
}
type Meeting struct {
	Entry
	Days []int
}

var Days = []string{"", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

func DayNames(days []int) string {
	names := []string{}
	for _, d := range days {
		if d >= 1 && d <= 7 {
			names = append(names, Days[d])
		}
	}
	if len(names) > 1 {
		return strings.Join(names[:len(names)-1], ", ") + " & " + names[len(names)-1]
	}
	return strings.Join(names, "")
}
func Time(n int) string {
	h := n / 60
	part := "AM"
	if h >= 12 {
		part = "PM"
	}
	h %= 12
	if h == 0 {
		h = 12
	}
	return fmt.Sprintf("%d:%02d %s", h, n%60, part)
}
func Merge(entries []Entry) []Meeting {
	out := []Meeting{}
	keys := map[string]int{}
	for _, e := range entries {
		key := fmt.Sprintf("%d|%d|%d|%s|%s|%s|%d|%d", e.OfferingID, e.FacultyID, e.RoomID, e.SubjectCode, e.Section, e.Kind, e.Start, e.End)
		index, ok := keys[key]
		if !ok {
			index = len(out)
			keys[key] = index
			out = append(out, Meeting{Entry: e})
		}
		exists := false
		for _, d := range out[index].Days {
			if d == e.Day {
				exists = true
			}
		}
		if !exists {
			out[index].Days = append(out[index].Days, e.Day)
		}
	}
	for i := range out {
		sort.Ints(out[i].Days)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		for k := 0; k < len(a.Days) && k < len(b.Days); k++ {
			if a.Days[k] != b.Days[k] {
				return a.Days[k] < b.Days[k]
			}
		}
		if len(a.Days) != len(b.Days) {
			return len(a.Days) < len(b.Days)
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.End != b.End {
			return a.End < b.End
		}
		if a.SubjectCode != b.SubjectCode {
			return a.SubjectCode < b.SubjectCode
		}
		return a.Section < b.Section
	})
	return out
}

// ShortDays is the explicit day set of a meeting; no timetable implies a fixed MWF pattern.
func ShortDays(days []int) string {
	labels := []string{"", "M", "T", "W", "Th", "F", "Sa", "Su"}
	out := []string{}
	for _, d := range days {
		if d >= 1 && d <= 7 {
			out = append(out, labels[d])
		}
	}
	return strings.Join(out, ", ")
}
func Matrix(kind string, entries []Entry) ([]string, [][]string) {
	heads := []string{"Time", "Subject / description", "Units", "Days", "Year / Section", "Instructor"}
	if kind == "faculty" {
		heads = []string{"Time", "Subject / description", "Units", "Days", "Room", "Year / Section"}
	}
	if kind == "sections" {
		heads = []string{"Time", "Subject / description", "Units", "Days", "Room", "Instructor"}
	}
	rows := [][]string{}
	for _, m := range Merge(entries) {
		sub := m.SubjectCode + " - " + m.SubjectTitle + " (" + m.Kind + ")"
		section := m.Section
		if m.YearLevel > 0 {
			section = fmt.Sprintf("%s / %s", yearLabel(m.YearLevel), section)
		}
		row := []string{Time(m.Start) + " - " + Time(m.End), sub, strconv.FormatFloat(m.Units, 'f', -1, 64), ShortDays(m.Days), section, m.Faculty}
		if kind == "faculty" {
			row = []string{Time(m.Start) + " - " + Time(m.End), sub, strconv.FormatFloat(m.Units, 'f', -1, 64), ShortDays(m.Days), m.Room, section}
		}
		if kind == "sections" {
			row = []string{Time(m.Start) + " - " + Time(m.End), sub, strconv.FormatFloat(m.Units, 'f', -1, 64), ShortDays(m.Days), m.Room, m.Faculty}
		}
		rows = append(rows, row)
	}
	return heads, rows
}
func TermLabel(frame Frame) string {
	if frame.Term != "" {
		return frame.Term
	}
	return strings.Trim(strings.Join([]string{frame.Semester, frame.AcademicYear}, " / "), " / ")
}

func yearLabel(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s year", n, suffix)
}
