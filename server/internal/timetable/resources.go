package timetable

import (
	"fmt"
	"sort"
)

type resourceSheet struct {
	Label   string
	Entries []Entry
}

// Each actual resource starts a separate sheet. Section identity includes the
// program and year even when older callers do not provide a stable SectionID.
func resourceSheets(kind string, entries []Entry, fallback string) []resourceSheet {
	groups := map[string]*resourceSheet{}
	for _, entry := range entries {
		key, label := "", ""
		switch kind {
		case "sections":
			key = fmt.Sprintf("%d|%s|%d|%s", entry.SectionID, entry.Program, entry.YearLevel, entry.Section)
			label = fmt.Sprintf("%s / %s / %s", entry.Program, yearLabel(entry.YearLevel), entry.Section)
		case "rooms":
			key = fmt.Sprintf("%d|%s", entry.RoomID, entry.Room)
			label = entry.Room
		case "faculty":
			key = fmt.Sprintf("%d|%s", entry.FacultyID, entry.Faculty)
			label = entry.Faculty
		}
		if groups[key] == nil {
			groups[key] = &resourceSheet{Label: label}
		}
		groups[key].Entries = append(groups[key].Entries, entry)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if groups[keys[i]].Label != groups[keys[j]].Label {
			return groups[keys[i]].Label < groups[keys[j]].Label
		}
		return keys[i] < keys[j]
	})
	out := []resourceSheet{}
	for _, key := range keys {
		out = append(out, *groups[key])
	}
	if len(out) == 0 {
		out = append(out, resourceSheet{Label: fallback})
	}
	return out
}
