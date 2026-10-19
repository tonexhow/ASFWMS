package timetable

import (
	"encoding/base64"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
)

type TableBlock struct {
	Title   string
	Headers []string
	Rows    [][]string
}

func sectionLabel(m Meeting) string {
	if m.YearLevel > 0 {
		return fmt.Sprintf("%s / %s - %s", m.Program, yearLabel(m.YearLevel), m.Section)
	}
	return m.Section
}

func subjectLabel(m Meeting) string {
	s := m.SubjectCode + " - " + m.SubjectTitle
	if m.Kind == "laboratory" {
		s += " (Lab)"
	}
	return s
}

// TableBlocks is retained for email/backward compatibility. Printable template
// grouping is handled by TemplateBlocks so presentation never changes entries.
func TableBlocks(kind string, entries []Entry) []TableBlock {
	meetings := Merge(entries)
	if kind == "faculty" {
		b := TableBlock{Headers: []string{"Time", "Subject", "No. of units", "Schedule", "Room", "Year and section"}, Rows: [][]string{}}
		for _, m := range meetings {
			b.Rows = append(b.Rows, []string{Time(m.Start) + " - " + Time(m.End), subjectLabel(m), strconv.FormatFloat(m.Units, 'f', -1, 64), ShortDays(m.Days), m.Room, sectionLabel(m)})
		}
		return []TableBlock{b}
	}
	out := []TableBlock{}
	for _, m := range meetings {
		label := DayNames(m.Days)
		if len(out) == 0 || out[len(out)-1].Title != label {
			heads := []string{"Time", "Subjects", "Units", "Schedule", "Yr. / Section", "Instructor"}
			if kind == "sections" {
				heads = []string{"Time", "Code", "Descriptive title", "Units", "Schedule", "Room #", "Instructor"}
			}
			out = append(out, TableBlock{Title: label, Headers: heads, Rows: [][]string{}})
		}
		clock := Time(m.Start) + " - " + Time(m.End)
		row := []string{clock, subjectLabel(m), strconv.FormatFloat(m.Units, 'f', -1, 64), ShortDays(m.Days), sectionLabel(m), m.Faculty}
		if kind == "sections" {
			title := m.SubjectTitle
			if m.Kind == "laboratory" {
				title += " (Lab)"
			}
			row = []string{clock, m.SubjectCode, title, strconv.FormatFloat(m.Units, 'f', -1, 64), ShortDays(m.Days), m.Room, m.Faculty}
		}
		out[len(out)-1].Rows = append(out[len(out)-1].Rows, row)
	}
	return out
}

func Departments(entries []Entry) string {
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Department != "" {
			seen[e.Department] = true
		}
	}
	a := []string{}
	for k := range seen {
		a = append(a, k)
	}
	sort.Strings(a)
	return strings.Join(a, " / ")
}

func Page(kind, title string, frame Frame, entries []Entry) string {
	return PageWithTemplate(kind, title, frame, entries, DefaultTemplate)
}

func scheduleLabel(kind string) string {
	switch kind {
	case "faculty":
		return "Faculty Schedule"
	case "rooms":
		return "Room Schedule"
	default:
		return "Section Schedule"
	}
}

func headerPairs(kind, title, department string, frame Frame) [][2]string {
	department = strings.TrimSpace(department)
	if department == "" {
		department = "Institution-wide"
	}
	semester := strings.TrimSpace(frame.Semester)
	if semester == "" {
		semester = frame.Term
	}
	ay := strings.TrimSpace(frame.AcademicYear)
	if ay == "" && frame.Term != "" {
		ay = frame.Term
	}
	resource := "Section"
	if kind == "faculty" {
		resource = "Instructor"
	} else if kind == "rooms" {
		resource = "Building / Room"
	}
	return [][2]string{{resource, title}, {"Department", department}, {"Semester", semester}, {"Academic Year", ay}}
}

func jpgDataURI(raw []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
}

func PageWithTemplate(kind, title string, frame Frame, entries []Entry, templateID string) string {
	esc := html.EscapeString
	spec, ok := ResolveTemplate(templateID)
	if !ok {
		spec, _ = ResolveTemplate(DefaultTemplate)
	}
	blocks, _ := TemplateBlocks(kind, entries, spec.ID)
	meta := headerPairs(kind, title, Departments(entries), frame)
	tone := strings.TrimPrefix(spec.ID, "split-")
	if spec.ID == "subject-list" {
		tone = "compact"
	}
	keys := []string{"time", "description", "units", "days", "room", "section"}
	if kind == "sections" {
		keys = []string{"time", "days", "code", "description", "units", "room", "faculty"}
	} else if kind == "rooms" {
		keys = []string{"time", "description", "units", "days", "section", "faculty"}
	}
	widths := TemplateWidths(kind, spec.ID)
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + esc(title) + `</title><style>` + SheetCSS + `</style></head><body><section class="sheet-page"><article class="schedule-sheet template-` + esc(tone) + `" data-sheet-kind="` + esc(kind) + `">`)
	b.WriteString(`<header class="schedule-sheet-masthead"><div class="sheet-masthead-grid"><div class="sheet-building-wrap"><img class="sheet-building" src="` + jpgDataURI(brandBuilding) + `" alt="Buenavista Community College building and seal"><img class="sheet-bcc-mark" src="` + jpgDataURI(brandBCCSeal) + `" alt="" aria-hidden="true"></div><div class="sheet-title-stack"><p class="sheet-minor">` + esc(InstitutionRepublic) + `</p><p class="sheet-minor">` + esc(InstitutionCommission) + `</p><h1 class="sheet-school-name">` + esc(InstitutionSchool) + `</h1><p class="sheet-place">` + esc(InstitutionAddress) + `</p><p class="sheet-tagline">&ldquo;` + esc(InstitutionTagline) + `&rdquo;</p><p class="sheet-contact">` + esc(InstitutionContact) + `</p></div><div class="sheet-logo-grid"><img class="sheet-logo-municipality" src="` + jpgDataURI(brandMunicipality) + `" alt="Municipality of Buenavista"><img class="sheet-logo-ched" src="` + jpgDataURI(brandCHED) + `" alt="Commission on Higher Education"><img class="sheet-logo-bagong" src="` + jpgDataURI(brandBagongPilipinas) + `" alt="Bagong Pilipinas"></div></div><div class="sheet-vision-mission"><p><strong>VISION:</strong><span>` + esc(InstitutionVision) + `</span></p><p><strong>MISSION:</strong><span>` + esc(InstitutionMission) + `</span></p></div></header>`)
	b.WriteString(`<h2 class="sheet-schedule-heading">` + esc(scheduleLabel(kind)) + `</h2><section class="sheet-meta-lines">`)
	for _, pair := range [][2][2]string{{meta[0], meta[2]}, {meta[1], meta[3]}} {
		b.WriteString(`<div class="sheet-meta-row">`)
		for _, item := range pair {
			b.WriteString(`<span><b>` + esc(item[0]) + `:</b> ` + esc(item[1]) + `</span>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)
	if frame.Version == 0 {
		b.WriteString(`<p class="sheet-status">PREVIEW - NOT AN OFFICIAL SCHEDULE</p>`)
	}
	for _, block := range blocks {
		b.WriteString(`<section class="sheet-day-group">`)
		if block.Title != "" {
			b.WriteString(`<h4>` + esc(block.Title) + `</h4>`)
		}
		b.WriteString(`<div class="schedule-table-scroll"><table class="schedule-sheet-table"><colgroup>`)
		for i, w := range widths {
			fmt.Fprintf(&b, `<col class="col-%s" style="width:%.6f%%">`, keys[i], w/756*100)
		}
		b.WriteString(`</colgroup><thead><tr>`)
		for i, h := range block.Headers {
			b.WriteString(`<th scope="col" class="col-` + keys[i] + `">` + esc(h) + `</th>`)
		}
		b.WriteString(`</tr></thead><tbody>`)
		for _, row := range block.Rows {
			b.WriteString(`<tr>`)
			for i, c := range row {
				b.WriteString(`<td class="col-` + keys[i] + `">` + esc(c) + `</td>`)
			}
			b.WriteString(`</tr>`)
		}
		if len(block.Rows) == 0 {
			fmt.Fprintf(&b, `<tr><td colspan="%d">No meetings in this timetable.</td></tr>`, len(block.Headers))
		}
		b.WriteString(`</tbody></table></div></section>`)
	}
	minutes := 0
	for _, e := range entries {
		minutes += e.End - e.Start
	}
	b.WriteString(`<footer class="schedule-sheet-footer"><p>Conforme: _______________________</p><p>Weekly contact: ` + strconv.FormatFloat(float64(minutes)/60, 'f', 2, 64) + ` hours</p><p>Printed ` + esc(frame.PrintedAt) + `</p></footer><p class="sheet-legend">Day headings are presentation groups only. Every row keeps the exact recorded days and time. Repeated rows within the same subject do not add extra units.</p><p class="sheet-credit">Designed and Developed by CITE and ICT Department</p></article></section></body></html>`)
	return b.String()
}

// SheetCSS is the same standalone stylesheet used by browser preview/print.
var SheetCSS string
