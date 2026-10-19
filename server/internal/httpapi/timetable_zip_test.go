package httpapi

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestCommittedSectionRoomFacultyZIP(t *testing.T) {
	h := newHarness(t)
	h.fixture()
	one := h.generate(1)
	h.commit(one, 200)
	two := h.generate(2)
	h.commit(two, 200)
	for _, view := range []string{"sections", "rooms", "faculty"} {
		_, data := h.call("GET", fmt.Sprintf("/api/timetable/zip?view=%s&period=1&department=1", view), nil, nil, 200)
		z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if e != nil || len(z.File) != 1 {
			t.Fatalf("%s ZIP entries=%d err=%v", view, len(z.File), e)
		}
		if !strings.HasSuffix(z.File[0].Name, ".pdf") {
			t.Fatalf("wrong timetable export: %s", z.File[0].Name)
		}
		p, e := z.File[0].Open()
		if e != nil {
			t.Fatal(e)
		}
		magic := make([]byte, 5)
		n, e := p.Read(magic)
		p.Close()
		if n != 5 || e != nil || string(magic) != "%PDF-" {
			t.Fatalf("%s ZIP has invalid PDF: %q %v", view, magic, e)
		}
	}
	h.call("GET", "/api/timetable/zip?view=sections&period=1&department=1&ids=2", nil, nil, 400)
	h.call("GET", "/api/timetable/zip?view=invalid&period=1", nil, nil, 400)
	h.call("GET", "/api/timetable/zip?view=sections&period=999", nil, nil, 404)
}
