package timetable

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	InstitutionRepublic     = "Republic of the Philippines"
	InstitutionCommission   = "Commission on Higher Education"
	InstitutionSchool       = "BUENAVISTA COMMUNITY COLLEGE"
	InstitutionAddress      = "Cangawa, Buenavista, Bohol"
	InstitutionTagline      = "Caring Your Future"
	InstitutionContact      = "(038) 500-0137; registrarofbcc@yahoo.com"
	InstitutionVisionTitle  = "VISION"
	InstitutionVision       = "A leading community college committed to transformative education that produces globally competent professionals."
	InstitutionMissionTitle = "MISSION"
	InstitutionMission      = "BCC is committed to providing transformative education, advancing relevant research, and engaging meaningful extension services for sustainable community development."
)

var brandMu sync.RWMutex

var brandBuilding, brandBCCSeal, brandCHED, brandMunicipality, brandBagongPilipinas []byte

// Source graphics live only in client/assets, not in the Go source tree.
func LoadBranding(clientAssetDir string) error {
	brandMu.Lock()
	defer brandMu.Unlock()
	files := []struct {
		name   string
		target *[]byte
	}{
		{"bcc-building.jpg", &brandBuilding}, {"bcc-seal.jpg", &brandBCCSeal}, {"ched.jpg", &brandCHED},
		{"buenavista-municipality.jpg", &brandMunicipality}, {"bagong-pilipinas.jpg", &brandBagongPilipinas},
	}
	for _, f := range files {
		b, e := os.ReadFile(filepath.Join(clientAssetDir, "timetable", f.name))
		if e != nil {
			return fmt.Errorf("schedule brand %s: %w", f.name, e)
		}
		*f.target = b
	}
	return nil
}
