package domain

import "fmt"

// YearLabel standardizes human-facing year levels without changing numeric
// year_level values stored in the academic schema.
func YearLabel(year int) string {
	if year <= 0 {
		return "Unknown year"
	}
	suffix := "th"
	if year%100 < 11 || year%100 > 13 {
		switch year % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s year", year, suffix)
}
