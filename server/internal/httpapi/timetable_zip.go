package httpapi

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"asfwms.local/scheduler/internal/domain"
)

// scheduleZIP is a read-only batch export of actual committed timetables.
// Every requested resource must have an authorized committed meeting in the
// selected period. It reuses the exact official PDF renderer and access rules.
func (a *App) scheduleZIP(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	view := params.Get("view")
	if view != "sections" && view != "rooms" && view != "faculty" {
		problem(w, 400, "VIEW", "Choose Sections, Rooms or Faculty.")
		return
	}
	period, err := strconv.Atoi(params.Get("period"))
	if err != nil || period <= 0 {
		problem(w, 400, "PERIOD", "Choose a valid academic period.")
		return
	}
	department := 0
	if params.Get("department") != "" {
		department, err = strconv.Atoi(params.Get("department"))
		if err != nil || department < 0 {
			problem(w, 400, "DEPARTMENT", "Choose a valid department.")
			return
		}
	}
	u := user(r)
	if u.Role == "scheduler" {
		if department != 0 && department != u.DepartmentID {
			deny(w)
			return
		}
		department = u.DepartmentID
	}
	s, _, err := domain.Load(r.Context(), a.Store.DB)
	if err != nil {
		problem(w, 503, "DATABASE", "Could not load committed timetables.")
		return
	}
	allowed := map[int]bool{}
	for _, m := range s.Meetings {
		if m.PeriodID != period || (department != 0 && m.DepartmentID != department) {
			continue
		}
		id := m.SectionID
		if view == "rooms" {
			id = m.RoomID
		} else if view == "faculty" {
			id = m.FacultyID
		}
		allowed[id] = true
	}
	ids := []int{}
	if values := params.Get("ids"); values != "" {
		seen := map[int]bool{}
		for _, part := range strings.Split(values, ",") {
			id, e := strconv.Atoi(strings.TrimSpace(part))
			if e != nil || id <= 0 || seen[id] || !allowed[id] {
				problem(w, 400, "SELECTION", "Select only timetables available to your department and chosen period.")
				return
			}
			seen[id] = true
			ids = append(ids, id)
		}
	} else {
		for id := range allowed {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		problem(w, 404, "EMPTY", "No committed timetables match the selected period.")
		return
	}
	if len(ids) > 250 {
		problem(w, 400, "LIMIT", "Select no more than 250 schedules per ZIP.")
		return
	}
	sort.Ints(ids)
	kind := view
	if view == "sections" {
		kind = "section"
	} else if view == "rooms" {
		kind = "room"
	}
	// Render completely before responding, so a renderer error cannot produce
	// a partially corrupt ZIP download.
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for _, id := range ids {
		q := url.Values{"view": {kind}, "period": {strconv.Itoa(period)}, "resource": {strconv.Itoa(id)}}
		if department != 0 {
			q.Set("department", strconv.Itoa(department))
		}
		if template := params.Get("template"); template != "" {
			q.Set("template", template)
		}
		request := r.Clone(r.Context())
		request.URL = &url.URL{Path: "/api/timetable/pdf", RawQuery: q.Encode()}
		recorder := httptest.NewRecorder()
		a.schedulePDF(recorder, request)
		if recorder.Code != http.StatusOK {
			archive.Close()
			problem(w, 422, "PDF", "At least one timetable could not be rendered. Nothing was downloaded.")
			return
		}
		if out.Len()+recorder.Body.Len() > 48<<20 {
			archive.Close()
			problem(w, 413, "LIMIT", "The selected PDFs exceed the batch size limit. Select fewer timetables.")
			return
		}
		filename := fmt.Sprintf("ASFWMS_%s_%04d.pdf", kind, id)
		target, err := archive.Create(filename)
		if err != nil {
			problem(w, 500, "ZIP", "Cannot create timetable ZIP.")
			return
		}
		if _, err = target.Write(recorder.Body.Bytes()); err != nil {
			problem(w, 500, "ZIP", "Cannot assemble timetable ZIP.")
			return
		}
	}
	if err = archive.Close(); err != nil {
		problem(w, 500, "ZIP", "Cannot finish timetable ZIP.")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=ASFWMS_%s_period_%d.zip", view, period))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out.Bytes())
}
