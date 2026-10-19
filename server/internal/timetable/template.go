package timetable

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const DefaultTemplate = "split-classic"

type TemplateSpec struct {
	ID, Name, Description, Mode string
	AccentR, AccentG, AccentB   float64
	SoftR, SoftG, SoftB         float64
}

var templateSpecs = map[string]TemplateSpec{
	"split-classic":  {ID: "split-classic", Name: "Official Classic", Description: "Workload-inspired institutional layout with restrained green accents and full weekday headings.", Mode: "split", AccentR: .10, AccentG: .35, AccentB: .23, SoftR: .93, SoftG: .97, SoftB: .94},
	"split-clean":    {ID: "split-clean", Name: "Contemporary Clean", Description: "Modern navy layout with stronger table contrast, generous white space, and full weekday headings.", Mode: "split", AccentR: .09, AccentG: .24, AccentB: .41, SoftR: .94, SoftG: .96, SoftB: .99},
	"split-minimal":  {ID: "split-minimal", Name: "Formal Minimal", Description: "Low-ink monochrome design for archival printing with minimal fills and clean rules.", Mode: "split", AccentR: .25, AccentG: .28, AccentB: .31, SoftR: .97, SoftG: .97, SoftB: .97},
	"split-emerald":  {ID: "split-emerald", Name: "Emerald Panel", Description: "Modern emerald table treatment with subtle alternating rows and full weekday headings.", Mode: "split", AccentR: .09, AccentG: .42, AccentB: .33, SoftR: .92, SoftG: .97, SoftB: .95},
	"split-slate":    {ID: "split-slate", Name: "Slate Signature", Description: "Professional slate-blue schedule sheet with crisp rules, balanced columns, and neutral printing contrast.", Mode: "split", AccentR: .21, AccentG: .31, AccentB: .43, SoftR: .94, SoftG: .96, SoftB: .98},
	"split-bamboo":   {ID: "split-bamboo", Name: "Bamboo Ledger", Description: "Warm bamboo-accented layout with softer dividers and a calmer ledger feel for section schedules.", Mode: "split", AccentR: .47, AccentG: .40, AccentB: .14, SoftR: .98, SoftG: .97, SoftB: .92},
	"split-maroon":   {ID: "split-maroon", Name: "Heritage Maroon", Description: "Classic maroon institutional typography, refined rules and archival contrast.", Mode: "split", AccentR: .48, AccentG: .18, AccentB: .25, SoftR: .99, SoftG: .95, SoftB: .96},
	"split-ocean":    {ID: "split-ocean", Name: "Coastal Blue", Description: "Bright university-blue headings, cool paper tint and modern sans-serif lettering.", Mode: "split", AccentR: .06, AccentG: .39, AccentB: .60, SoftR: .93, SoftG: .97, SoftB: .99},
	"split-violet":   {ID: "split-violet", Name: "Academic Violet", Description: "Purple faculty ledger with high-contrast white headings and serif titles.", Mode: "split", AccentR: .34, AccentG: .25, AccentB: .51, SoftR: .96, SoftG: .95, SoftB: .99},
	"split-sunrise":  {ID: "split-sunrise", Name: "Sunrise Ledger", Description: "Warm terracotta highlights and an inviting ledger layout.", Mode: "split", AccentR: .63, AccentG: .29, AccentB: .16, SoftR: .99, SoftG: .96, SoftB: .94},
	"split-graphite": {ID: "split-graphite", Name: "Graphite Type", Description: "Minimal ink, monospaced table for easy checking and precise alignment.", Mode: "split", AccentR: .18, AccentG: .23, AccentB: .27, SoftR: .95, SoftG: .96, SoftB: .97},
	"subject-list":   {ID: "subject-list", Name: "Compact Subject List", Description: "Space-efficient ledger layout with one continuous subject table and actual days/times on each row.", Mode: "list", AccentR: .18, AccentG: .23, AccentB: .30, SoftR: .96, SoftG: .97, SoftB: .98},
}

func ResolveTemplate(id string) (TemplateSpec, bool) {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		id = DefaultTemplate
	}
	t, ok := templateSpecs[id]
	return t, ok
}

func TemplateList() []TemplateSpec {
	ids := []string{"split-classic", "split-clean", "split-minimal", "split-emerald", "split-slate", "split-bamboo", "split-maroon", "split-ocean", "split-violet", "split-sunrise", "split-graphite", "subject-list"}
	out := make([]TemplateSpec, 0, len(ids))
	for _, id := range ids {
		out = append(out, templateSpecs[id])
	}
	return out
}

func templateFamily(days []int) (int, string) {
	if len(days) == 0 {
		return 4, "M-F / Flexible"
	}
	mwf, tth, sat := true, true, true
	for _, d := range days {
		if d != 1 && d != 3 && d != 5 {
			mwf = false
		}
		if d != 2 && d != 4 {
			tth = false
		}
		if d != 6 {
			sat = false
		}
	}
	switch {
	case mwf:
		return 1, "Monday, Wednesday, Friday"
	case tth:
		return 2, "Tuesday, Thursday"
	case sat:
		return 3, "Saturday (optional)"
	default:
		return 4, "M-F / Flexible"
	}
}

func templateMeetings(entries []Entry, mode string) []Meeting {
	rows := Merge(entries)
	sort.SliceStable(rows, func(i, j int) bool {
		if mode != "list" {
			ai, _ := templateFamily(rows[i].Days)
			aj, _ := templateFamily(rows[j].Days)
			if ai != aj {
				return ai < aj
			}
		}
		if rows[i].Start != rows[j].Start {
			return rows[i].Start < rows[j].Start
		}
		if c := compareDaySlices(rows[i].Days, rows[j].Days); c != 0 {
			return c < 0
		}
		if rows[i].SubjectCode != rows[j].SubjectCode {
			return rows[i].SubjectCode < rows[j].SubjectCode
		}
		return rows[i].Section < rows[j].Section
	})
	return rows
}

func compareDaySlices(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func templateHeaders(kind string) []string {
	if kind == "sections" {
		return []string{"Time", "Days", "Code", "Descriptive title", "Units", "Room", "Instructor"}
	}
	if kind == "faculty" {
		return []string{"Time", "Subject", "Units", "Schedule", "Room", "Year / Section"}
	}
	return []string{"Time", "Subject", "Units", "Schedule", "Year / Section", "Instructor"}
}

func templateRow(kind string, m Meeting) []string {
	clock := Time(m.Start) + " - " + Time(m.End)
	days := ShortDays(m.Days)
	units := strconv.FormatFloat(m.Units, 'f', -1, 64)
	if kind == "sections" {
		title := m.SubjectTitle
		if m.Kind == "laboratory" {
			title += " (Lab)"
		}
		return []string{clock, days, m.SubjectCode, title, units, m.Room, m.Faculty}
	}
	if kind == "faculty" {
		return []string{clock, subjectLabel(m), units, days, m.Room, sectionLabel(m)}
	}
	return []string{clock, subjectLabel(m), units, days, sectionLabel(m), m.Faculty}
}

func TemplateBlocks(kind string, entries []Entry, templateID string) ([]TableBlock, error) {
	spec, ok := ResolveTemplate(templateID)
	if !ok {
		return nil, fmt.Errorf("unknown schedule template")
	}
	rows := templateMeetings(entries, spec.Mode)
	heads := templateHeaders(kind)
	if spec.Mode == "list" {
		b := TableBlock{Headers: heads, Rows: [][]string{}}
		for _, m := range rows {
			b.Rows = append(b.Rows, templateRow(kind, m))
		}
		return []TableBlock{b}, nil
	}
	blocks := []TableBlock{}
	by := map[string]int{}
	for _, m := range rows {
		_, title := templateFamily(m.Days)
		idx, found := by[title]
		if !found {
			idx = len(blocks)
			by[title] = idx
			blocks = append(blocks, TableBlock{Title: title, Headers: heads, Rows: [][]string{}})
		}
		blocks[idx].Rows = append(blocks[idx].Rows, templateRow(kind, m))
	}
	return blocks, nil
}

func TemplateWidths(kind, templateID string) []float64 {
	// Short columns stay deliberately narrow while descriptive text receives
	// most of the landscape page. All returned widths sum to 756 pt, matching
	// the printable area of US Letter / short bond landscape with 18 pt margins.
	if kind == "sections" {
		return []float64{108, 48, 64, 248, 44, 112, 132}
	}
	if kind == "faculty" {
		return []float64{105, 270, 46, 70, 115, 150}
	}
	return []float64{105, 280, 46, 70, 145, 110}
}
