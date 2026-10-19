package timetable

// A small, dependency-free PDF writer using PDF's built-in fonts and embedded
// JPEG branding assets. The renderer is presentation-only: it never changes
// meetings, units, faculty, rooms, or schedule logic.
import (
	"bytes"
	"fmt"
	"image/jpeg"
	"strings"
	"unicode"
)

func pdfString(s string) (string, error) {
	var b strings.Builder
	extra := map[rune]byte{'\u20ac': 128, '\u201a': 130, '\u0192': 131, '\u201e': 132, '\u2026': 133, '\u2020': 134, '\u2021': 135, '\u02c6': 136, '\u2030': 137, '\u0160': 138, '\u2039': 139, '\u0152': 140, '\u017d': 142, '\u2018': 145, '\u2019': 146, '\u201c': 147, '\u201d': 148, '\u2022': 149, '\u2013': 150, '\u2014': 151, '\u02dc': 152, '\u2122': 153, '\u0161': 154, '\u203a': 155, '\u0153': 156, '\u017e': 158, '\u0178': 159}
	for _, r := range s {
		var c byte
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			c = ' '
		case r >= 32 && r <= 126 || r >= 160 && r <= 255:
			c = byte(r)
		default:
			var ok bool
			c, ok = extra[r]
			if !ok {
				return "", fmt.Errorf("PDF font cannot represent %q; use Print / Save as PDF in your browser for this text", r)
			}
		}
		if c == '(' || c == ')' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String(), nil
}

func glyphWidth(r rune) float64 {
	if strings.ContainsRune("ilI.,'!:;| ", r) {
		return .29
	}
	if strings.ContainsRune("mwMW@%", r) {
		return .95
	}
	if unicode.IsUpper(r) {
		return .75
	}
	return .61
}

func textWidth(s string, size float64) float64 {
	width := 0.0
	for _, r := range s {
		width += glyphWidth(r) * size
	}
	return width
}

func wrapPDF(s string, width, size float64) []string {
	out := []string{}
	line := ""
	used := 0.0
	for _, word := range strings.Fields(s) {
		w := textWidth(word, size)
		space := 0.0
		if line != "" {
			space = .29 * size
		}
		if line != "" && used+space+w > width {
			out = append(out, line)
			line = ""
			used = 0
			space = 0
		}
		if line != "" {
			line += " "
			used += space
		}
		// Keep unusually long tokens safe instead of allowing them to overrun a cell.
		for _, r := range word {
			rw := glyphWidth(r) * size
			if line != "" && used+rw > width {
				out = append(out, line)
				line = ""
				used = 0
			}
			line += string(r)
			used += rw
		}
	}
	if line != "" || len(out) == 0 {
		out = append(out, line)
	}
	return out
}

type pdfPalette struct {
	HeaderFill [3]float64
	HeaderText [3]float64
	AltFill    [3]float64
	Rule       [3]float64
	GroupText  [3]float64
	GroupStyle string
}

func paletteForTemplate(id string) pdfPalette {
	switch id {
	case "split-clean":
		return pdfPalette{HeaderFill: [3]float64{.09, .24, .41}, HeaderText: [3]float64{1, 1, 1}, AltFill: [3]float64{.965, .978, .992}, Rule: [3]float64{.40, .49, .58}, GroupText: [3]float64{.09, .24, .41}, GroupStyle: "left"}
	case "split-minimal":
		return pdfPalette{HeaderFill: [3]float64{.975, .975, .975}, HeaderText: [3]float64{.18, .20, .22}, AltFill: [3]float64{.988, .988, .988}, Rule: [3]float64{.48, .50, .52}, GroupText: [3]float64{.24, .26, .28}, GroupStyle: "plain"}
	case "split-emerald":
		return pdfPalette{HeaderFill: [3]float64{.10, .42, .33}, HeaderText: [3]float64{1, 1, 1}, AltFill: [3]float64{.955, .982, .970}, Rule: [3]float64{.36, .52, .46}, GroupText: [3]float64{.08, .36, .28}, GroupStyle: "double"}
	case "split-slate":
		return pdfPalette{HeaderFill: [3]float64{.918, .941, .965}, HeaderText: [3]float64{.129, .235, .349}, AltFill: [3]float64{.958, .970, .984}, Rule: [3]float64{.49, .56, .64}, GroupText: [3]float64{.18, .28, .39}, GroupStyle: "center"}
	case "split-bamboo":
		return pdfPalette{HeaderFill: [3]float64{.957, .941, .875}, HeaderText: [3]float64{.325, .278, .118}, AltFill: [3]float64{.989, .984, .948}, Rule: [3]float64{.66, .61, .43}, GroupText: [3]float64{.38, .31, .10}, GroupStyle: "center"}
	case "split-maroon":
		return pdfPalette{HeaderFill: [3]float64{.48, .18, .25}, HeaderText: [3]float64{1, 1, 1}, AltFill: [3]float64{.99, .96, .97}, Rule: [3]float64{.69, .53, .56}, GroupText: [3]float64{.45, .16, .23}, GroupStyle: "center"}
	case "split-ocean":
		return pdfPalette{HeaderFill: [3]float64{.06, .39, .60}, HeaderText: [3]float64{1, 1, 1}, AltFill: [3]float64{.94, .98, .99}, Rule: [3]float64{.47, .64, .76}, GroupText: [3]float64{.06, .35, .54}, GroupStyle: "left"}
	case "split-violet":
		return pdfPalette{HeaderFill: [3]float64{.34, .25, .51}, HeaderText: [3]float64{1, 1, 1}, AltFill: [3]float64{.97, .96, .99}, Rule: [3]float64{.62, .56, .73}, GroupText: [3]float64{.32, .22, .49}, GroupStyle: "center"}
	case "split-sunrise":
		return pdfPalette{HeaderFill: [3]float64{.96, .92, .88}, HeaderText: [3]float64{.48, .22, .12}, AltFill: [3]float64{.99, .97, .95}, Rule: [3]float64{.72, .60, .52}, GroupText: [3]float64{.59, .29, .16}, GroupStyle: "left"}
	case "split-graphite":
		return pdfPalette{HeaderFill: [3]float64{.18, .23, .27}, HeaderText: [3]float64{1, 1, 1}, AltFill: [3]float64{.965, .97, .98}, Rule: [3]float64{.48, .53, .57}, GroupText: [3]float64{.18, .23, .27}, GroupStyle: "plain"}
	case "subject-list":
		return pdfPalette{HeaderFill: [3]float64{.19, .25, .31}, HeaderText: [3]float64{1, 1, 1}, AltFill: [3]float64{.965, .972, .980}, Rule: [3]float64{.43, .49, .55}, GroupText: [3]float64{.19, .25, .31}, GroupStyle: "plain"}
	default:
		return pdfPalette{HeaderFill: [3]float64{.94, .97, .95}, HeaderText: [3]float64{.08, .26, .18}, AltFill: [3]float64{.982, .990, .985}, Rule: [3]float64{.42, .52, .47}, GroupText: [3]float64{.08, .30, .21}, GroupStyle: "center"}
	}
}

func PDF(kind, title string, frame Frame, entries []Entry) ([]byte, error) {
	return PDFWithTemplate(kind, title, frame, entries, DefaultTemplate)
}

// PDFWithTemplate renders short bond / US Letter in landscape orientation.
// The first page follows the institution's workload-style header. Continuation
// pages use a compact header so schedules do not waste an entire page on branding.
func PDFWithTemplate(kind, title string, frame Frame, entries []Entry, templateID string) ([]byte, error) {
	brandMu.RLock()
	defer brandMu.RUnlock()
	if kind != "sections" && kind != "rooms" && kind != "faculty" {
		return nil, fmt.Errorf("unknown timetable view")
	}
	spec, ok := ResolveTemplate(templateID)
	if !ok {
		spec, _ = ResolveTemplate(DefaultTemplate)
	}
	blocks, err := TemplateBlocks(kind, entries, spec.ID)
	if err != nil {
		return nil, err
	}
	widths := TemplateWidths(kind, spec.ID)
	palette := paletteForTemplate(spec.ID)
	// Standard PDF fonts are embedded by reference (no external assets). The
	// chosen family applies to table text and institutional headings alike.
	bodyFont, boldFont, italicFont := "F1", "F2", "F3"
	switch spec.ID {
	case "split-maroon", "split-violet", "split-bamboo":
		bodyFont, boldFont, italicFont = "F4", "F5", "F6" // Times
	case "split-graphite":
		bodyFont, boldFont, italicFont = "F7", "F8", "F7" // Courier
	}

	const (
		pageWidth   = 792.0
		pageHeight  = 612.0
		left        = 18.0
		right       = 774.0
		tableWidth  = 756.0
		bodyBottom  = 569.0
		footerLineY = 575.0
		footerTextY = 582.0
	)

	dept := Departments(entries)
	if dept == "" {
		dept = "Institution-wide"
	}
	meta := headerPairs(kind, title, dept, frame)
	images, err := pdfBrandImages()
	if err != nil {
		return nil, err
	}

	type pageDef struct{ content string }
	pages := []pageDef{}
	var content bytes.Buffer
	var failure error
	y := 0.0
	pageNumber := 0

	setFill := func(r, g, b float64) { fmt.Fprintf(&content, "%.3f %.3f %.3f rg\n", r, g, b) }
	setStroke := func(r, g, b float64) { fmt.Fprintf(&content, "%.3f %.3f %.3f RG\n", r, g, b) }
	line := func(x1, y1, x2, y2, width float64) {
		fmt.Fprintf(&content, "q %.2f w %.2f %.2f m %.2f %.2f l S Q\n", width, x1, pageHeight-y1, x2, pageHeight-y2)
	}
	fill := func(r, g, b, x, top, w, h float64) {
		fmt.Fprintf(&content, "q %.3f %.3f %.3f rg %.2f %.2f %.2f %.2f re f Q\n", r, g, b, x, pageHeight-top-h, w, h)
	}
	strokeRect := func(x, top, w, h, width float64) {
		fmt.Fprintf(&content, "q %.2f w %.2f %.2f %.2f %.2f re S Q\n", width, x, pageHeight-top-h, w, h)
	}
	drawText := func(x, top, size float64, font, s string) {
		switch font {
		case "F1":
			font = bodyFont
		case "F2":
			font = boldFont
		case "F3":
			font = italicFont
		}
		if failure != nil {
			return
		}
		v, e := pdfString(s)
		if e != nil {
			failure = e
			return
		}
		fmt.Fprintf(&content, "BT /%s %.2f Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n", font, size, x, pageHeight-top-size, v)
	}
	centerText := func(x, top, width, size float64, font, s string) {
		drawText(x+(width-textWidth(s, size))/2, top, size, font, s)
	}
	wrapText := func(x, top, width, size float64, font, s string, leading float64) float64 {
		lines := wrapPDF(s, width, size)
		for i, row := range lines {
			drawText(x, top+float64(i)*leading, size, font, row)
		}
		return float64(len(lines)) * leading
	}
	drawImage := func(name string, x, top, w, h float64) {
		fmt.Fprintf(&content, "q %.2f 0 0 %.2f %.2f %.2f cm /%s Do Q\n", w, h, x, pageHeight-top-h, name)
	}

	footer := func() {
		setStroke(.62, .67, .71)
		line(left, footerLineY, right, footerLineY, .45)
		setFill(.30, .34, .38)
		label := "Selected schedule"
		if frame.Version == 0 {
			label = "Preview"
		}
		drawText(left, footerTextY, 6.9, "F1", fmt.Sprintf("ASFWMS | %s | %s", spec.Name, label))
		centerText(left, footerTextY, tableWidth, 6.9, "F1", fmt.Sprintf("Page %d", pageNumber))
		rightText := "Printed " + frame.PrintedAt
		drawText(right-textWidth(rightText, 6.9), footerTextY, 6.9, "F1", rightText)
	}
	finish := func() {
		footer()
		pages = append(pages, pageDef{content.String()})
	}

	firstHeader := func() {
		content.Reset()
		pageNumber++
		y = 14
		// Institutional banner: building+BCC seal | official text | municipal/CHED/BP logos.
		fill(1, 1, 1, left, y, tableWidth, 113)
		setStroke(.58, .68, .62)
		strokeRect(left, y, tableWidth, 113, .55)
		drawImage("ImBuild", left+7, y+7, 144, 68)
		// The BCC seal intentionally overlaps the building edge, matching the
		// supplied institutional header while keeping both images crisp.
		drawImage("ImBCC", left+124, y+12, 58, 58)

		centerX, centerW := left+184, 386.0
		setFill(.15, .20, .18)
		centerText(centerX, y+5, centerW, 9.0, "F1", InstitutionRepublic)
		centerText(centerX, y+16, centerW, 9.0, "F1", InstitutionCommission)
		setFill(palette.GroupText[0], palette.GroupText[1], palette.GroupText[2])
		centerText(centerX, y+29, centerW, 15.6, "F2", InstitutionSchool)
		setFill(.17, .22, .20)
		centerText(centerX, y+48, centerW, 9.5, "F1", InstitutionAddress)
		centerText(centerX, y+60, centerW, 10.4, "F3", `"`+InstitutionTagline+`"`)
		centerText(centerX, y+72, centerW, 7.8, "F1", InstitutionContact)

		// Right side intentionally contains only the external institutional marks.
		logoY := y + 10
		logoSize := 54.0
		logoGap := 6.0
		logoX := right - (logoSize*3 + logoGap*2) - 5
		for i, name := range []string{"ImMunicipality", "ImCHED", "ImBagong"} {
			drawImage(name, logoX+float64(i)*(logoSize+logoGap), logoY, logoSize, logoSize)
		}

		// Vision and Mission follow the sample as two full-width rows.
		stripY := y + 84
		setStroke(.72, .78, .74)
		line(left+7, stripY, right-7, stripY, .35)
		line(left+7, stripY+24, right-7, stripY+24, .35)
		setFill(.15, .22, .18)
		drawText(left+14, stripY+3.5, 7.2, "F2", InstitutionVisionTitle+":")
		wrapText(left+59, stripY+3.5, tableWidth-82, 6.8, "F1", InstitutionVision, 7.5)
		drawText(left+14, stripY+13.0, 7.2, "F2", InstitutionMissionTitle+":")
		wrapText(left+59, stripY+13.0, tableWidth-82, 6.8, "F1", InstitutionMission, 7.5)
		y += 120

		setFill(.12, .20, .17)
		centerText(left, y, tableWidth, 14.2, "F2", scheduleLabel(kind))
		y += 21

		// Workload-style metadata: two rows, two columns, compact and balanced.
		gap := 28.0
		colW := (tableWidth - gap) / 2
		leftX := left
		rightX := left + colW + gap

		pairs := [][2][2]string{{meta[0], meta[2]}, {meta[1], meta[3]}}
		for _, pair := range pairs {
			linesByColumn := [2][]string{}
			rowH := 23.0
			for i, field := range pair {
				linesByColumn[i] = wrapPDF(field[1], colW-78, 8.8)
				h := 8.0 + float64(len(linesByColumn[i]))*9.8
				if h > rowH {
					rowH = h
				}
			}
			for colIndex, field := range pair {
				x := leftX
				if colIndex == 1 {
					x = rightX
				}
				setFill(.16, .20, .24)
				drawText(x, y+3.5, 8.4, "F2", field[0]+":")
				setFill(.12, .16, .20)
				for i, value := range linesByColumn[colIndex] {
					drawText(x+74, y+3.2+float64(i)*9.8, 8.8, "F1", value)
				}
				setStroke(.39, .45, .50)
				line(x, y+rowH-3, x+colW, y+rowH-3, .45)
			}
			y += rowH
		}

		if frame.Version == 0 {
			fill(.957, .973, .988, left, y+1, tableWidth, 14)
			setFill(.15, .34, .48)
			centerText(left, y+4, tableWidth, 7.6, "F2", "PREVIEW - NOT AN OFFICIAL SCHEDULE")
			y += 19
		} else {
			y += 5
		}
	}

	continuationHeader := func() {
		content.Reset()
		pageNumber++
		y = 16
		setFill(palette.GroupText[0], palette.GroupText[1], palette.GroupText[2])
		drawText(left, y, 10.2, "F2", InstitutionSchool)
		setFill(.20, .25, .29)
		label := scheduleLabel(kind) + " - " + title
		titleLines := wrapPDF(label, tableWidth*0.62, 8.8)
		termLines := wrapPDF(TermLabel(frame), tableWidth*0.34, 7.6)
		y += 16
		for i, v := range titleLines {
			drawText(left, y+float64(i)*10, 8.8, "F2", v)
		}
		for i, v := range termLines {
			drawText(right-textWidth(v, 7.6), y+float64(i)*10, 7.6, "F1", v)
		}
		n := len(titleLines)
		if len(termLines) > n {
			n = len(termLines)
		}
		y += float64(n)*10 + 5
		setStroke(palette.Rule[0], palette.Rule[1], palette.Rule[2])
		line(left, y, right, y, 1)
		y += 8
	}

	drawGroupTitle := func(title string, continued bool) {
		if title == "" {
			return
		}
		if continued {
			title += " (continued)"
		}
		setFill(palette.GroupText[0], palette.GroupText[1], palette.GroupText[2])
		switch palette.GroupStyle {
		case "left":
			drawText(left, y+2, 10.0, "F2", title)
			setStroke(palette.Rule[0], palette.Rule[1], palette.Rule[2])
			line(left, y+15, right, y+15, .65)
		case "double":
			setStroke(palette.Rule[0], palette.Rule[1], palette.Rule[2])
			line(left, y+1, right, y+1, .45)
			centerText(left, y+4, tableWidth, 10.0, "F2", title)
			line(left, y+17, right, y+17, .45)
		case "plain":
			centerText(left, y+3, tableWidth, 9.7, "F2", title)
		default:
			centerText(left, y+3, tableWidth, 10.0, "F2", title)
			setStroke(palette.Rule[0], palette.Rule[1], palette.Rule[2])
			line(left+110, y+17, right-110, y+17, .55)
		}
		y += 21
	}

	centerColumn := func(col int) bool {
		if kind == "sections" {
			return col == 0 || col == 1 || col == 2 || col == 4
		}
		return col == 0 || col == 2 || col == 3
	}

	drawTableHeader := func(headers []string) {
		rowH := 21.0
		fill(palette.HeaderFill[0], palette.HeaderFill[1], palette.HeaderFill[2], left, y, tableWidth, rowH)
		setStroke(palette.Rule[0], palette.Rule[1], palette.Rule[2])
		x := left
		for i, h := range headers {
			lines := wrapPDF(strings.ToUpper(h), widths[i]-8, 7.6)
			total := float64(len(lines)) * 8
			for j, value := range lines {
				setFill(palette.HeaderText[0], palette.HeaderText[1], palette.HeaderText[2])
				centerText(x+4, y+(rowH-total)/2+1+float64(j)*8, widths[i]-8, 7.6, "F2", value)
			}
			line(x, y, x, y+rowH, .45)
			x += widths[i]
		}
		line(x, y, x, y+rowH, .45)
		line(left, y, right, y, .45)
		line(left, y+rowH, right, y+rowH, .55)
		y += rowH
	}

	drawRow := func(row []string, index int) {
		fontSize := 8.15
		leading := 9.0
		cells := make([][]string, len(row))
		maxLines := 1
		for i, cell := range row {
			cells[i] = wrapPDF(cell, widths[i]-10, fontSize)
			if len(cells[i]) > maxLines {
				maxLines = len(cells[i])
			}
		}
		rowH := float64(maxLines)*leading + 9
		if rowH < 22 {
			rowH = 22
		}
		if index%2 == 1 {
			fill(palette.AltFill[0], palette.AltFill[1], palette.AltFill[2], left, y, tableWidth, rowH)
		}
		setStroke(palette.Rule[0], palette.Rule[1], palette.Rule[2])
		x := left
		for i, cell := range cells {
			contentTop := y + (rowH-float64(len(cell))*leading)/2 + .5
			for j, value := range cell {
				setFill(.08, .11, .14)
				if centerColumn(i) {
					centerText(x+5, contentTop+float64(j)*leading, widths[i]-10, fontSize, "F1", value)
				} else {
					drawText(x+5, contentTop+float64(j)*leading, fontSize, "F1", value)
				}
			}
			line(x, y, x, y+rowH, .42)
			x += widths[i]
		}
		line(x, y, x, y+rowH, .42)
		line(left, y+rowH, right, y+rowH, .42)
		y += rowH
	}

	for _, resource := range resourceSheets(kind, entries, title) {
		entries = resource.Entries
		title = resource.Label
		meta = headerPairs(kind, title, Departments(entries), frame)
		blocks, err = TemplateBlocks(kind, entries, spec.ID)
		if err != nil {
			return nil, err
		}
		firstHeader()
		for _, block := range blocks {
			if y+42 > bodyBottom {
				finish()
				continuationHeader()
			}
			drawGroupTitle(block.Title, false)
			drawTableHeader(block.Headers)
			for rowIndex, row := range block.Rows {
				// Pre-compute the row height to decide if a continuation page is needed.
				maxLines := 1
				for i, cell := range row {
					if n := len(wrapPDF(cell, widths[i]-10, 8.15)); n > maxLines {
						maxLines = n
					}
				}
				rowH := float64(maxLines)*9 + 9
				if rowH < 22 {
					rowH = 22
				}
				if y+rowH > bodyBottom {
					finish()
					continuationHeader()
					drawGroupTitle(block.Title, true)
					drawTableHeader(block.Headers)
				}
				if y+rowH > bodyBottom {
					return nil, fmt.Errorf("one timetable row is too tall for Letter landscape; shorten the record text or use browser Print for this exceptional row")
				}
				drawRow(row, rowIndex)
			}
			y += 8
		}

		minutes := 0
		for _, e := range entries {
			minutes += e.End - e.Start
		}
		notes := []string{
			fmt.Sprintf("Conforme: _______________________     Weekly contact: %.2f hours", float64(minutes)/60),
			"Day headings are presentation groups only. Each row keeps the exact recorded days and time; repeated rows do not add units.",
			"Designed and Developed by CITE and ICT Department.",
		}
		if len(entries) == 0 {
			notes = append([]string{"No meetings in this timetable."}, notes...)
		}
		if y+float64(len(notes))*8 > bodyBottom {
			finish()
			continuationHeader()
		}
		setFill(.26, .31, .35)
		for _, note := range notes {
			drawText(left, y, 6.6, "F1", note)
			y += 8
		}
		finish()
	}

	if failure != nil {
		return nil, failure
	}

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Oblique /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Times-Bold /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Times-Italic /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold /Encoding /WinAnsiEncoding >>",
	}
	imageIDs := map[string]int{}
	for _, img := range images {
		id := len(objects) + 1
		imageIDs[img.name] = id
		objects = append(objects, fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream", img.width, img.height, len(img.data), string(img.data)))
	}
	xObjects := []string{}
	for _, img := range images {
		xObjects = append(xObjects, fmt.Sprintf("/%s %d 0 R", img.name, imageIDs[img.name]))
	}
	resourceXObjects := strings.Join(xObjects, " ")
	kids := []string{}
	for _, page := range pages {
		id := len(objects) + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", id))
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R /F4 6 0 R /F5 7 0 R /F6 8 0 R /F7 9 0 R /F8 10 0 R >> /XObject << %s >> >> /Contents %d 0 R >>", pageWidth, pageHeight, resourceXObjects, id+1),
			fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len([]byte(page.content)), page.content),
		)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(pages), strings.Join(kids, " "))

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes(), nil
}

type pdfImage struct {
	name          string
	data          []byte
	width, height int
}

func pdfBrandImages() ([]pdfImage, error) {
	pairs := []struct {
		name string
		data []byte
	}{
		{"ImBuild", brandBuilding},
		{"ImBCC", brandBCCSeal},
		{"ImMunicipality", brandMunicipality},
		{"ImCHED", brandCHED},
		{"ImBagong", brandBagongPilipinas},
	}
	out := make([]pdfImage, 0, len(pairs))
	for _, pair := range pairs {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(pair.data))
		if err != nil {
			return nil, err
		}
		out = append(out, pdfImage{name: pair.name, data: pair.data, width: cfg.Width, height: cfg.Height})
	}
	return out, nil
}
