package domain

import (
	"errors"
	"fmt"
)

// ResolveSectionCurriculum maps a section to the active curriculum for its
// program, year and academic-period semester. Scoped curricula take precedence;
// a unique legacy curriculum is accepted only when it actually contains the
// requested year. An ambiguous or missing mapping must be fixed in Academic.
func ResolveSectionCurriculum(s Snapshot, programID, yearLevel, periodID int) (int, error) {
	if programID <= 0 || yearLevel < 1 || yearLevel > 6 || periodID <= 0 {
		return 0, errors.New("choose a program, academic period and valid year level")
	}
	validProgram := false
	for _, p := range s.Programs {
		if p.ID == programID && p.Active == 1 {
			validProgram = true
			break
		}
	}
	if !validProgram {
		return 0, errors.New("choose an active program")
	}
	semester := ""
	for _, period := range s.Periods {
		if period.ID == periodID && period.Active == 1 {
			semester = period.Semester
			break
		}
	}
	if semester == "" {
		return 0, errors.New("choose an active academic period with a semester")
	}
	var scoped, legacy []int
	for _, c := range s.Curricula {
		if c.ProgramID != programID || c.Active != 1 {
			continue
		}
		if c.YearLevel == yearLevel && c.Semester != "" && SemestersMatch(c.Semester, semester) {
			scoped = append(scoped, c.ID)
		}
		if c.YearLevel == 0 && c.Semester == "" {
			for _, link := range s.Mappings {
				if link.CurriculumID == c.ID && link.Year == yearLevel {
					legacy = append(legacy, c.ID)
					break
				}
			}
		}
	}
	if len(scoped) == 1 {
		return scoped[0], nil
	}
	if len(scoped) > 1 {
		return 0, fmt.Errorf("multiple active curricula match this program, year and semester; resolve the duplicate before saving the section")
	}
	if len(legacy) == 1 {
		return legacy[0], nil
	}
	if len(legacy) > 1 {
		return 0, errors.New("multiple older curricula match this program and year; create a semester-specific curriculum first")
	}
	return 0, fmt.Errorf("no active curriculum matches this program, Year %d and %s; create its curriculum and link subjects first", yearLevel, semester)
}
