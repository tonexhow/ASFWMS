package storage

import (
	"asfwms.local/scheduler/internal/facilities"
	"database/sql"
	"fmt"
	"strings"
)

type FacilityIssue struct{ Code, Message string }

// AuditFacilities never writes or deactivates records; only a human can
// confirm that names referring to the same physical space are aliases.
func AuditFacilities(db *sql.DB) ([]FacilityIssue, error) {
	rows, e := db.Query(`SELECT r.id,r.code,r.name,r.kind,r.active,COALESCE(r.building_id,0),
 COALESCE(b.name,''),COALESCE(b.active,0),COALESCE(b.id,0)
 FROM rooms r LEFT JOIN buildings b ON b.id=r.building_id ORDER BY r.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	issues := []FacilityIssue{}
	type found struct {
		id                   int
		code, name, building string
	}
	seen := map[string]found{}
	for rows.Next() {
		var id, active, buildingID, buildingActive, present int
		var code, name, kind, building string
		if e = rows.Scan(&id, &code, &name, &kind, &active, &buildingID, &building, &buildingActive, &present); e != nil {
			return nil, e
		}
		if active != 1 {
			continue
		}
		if present == 0 {
			issues = append(issues, FacilityIssue{"UNASSIGNED_BUILDING", fmt.Sprintf("active room %s (#%d) is missing a valid building", code, id)})
			continue
		}
		if buildingActive != 1 {
			issues = append(issues, FacilityIssue{"INACTIVE_BUILDING", fmt.Sprintf("active room %s (#%d) belongs to inactive building %s", code, id, building)})
			continue
		}
		key := facilities.Key(buildingID, id, kind, name)
		if earlier, ok := seen[key]; ok {
			issues = append(issues, FacilityIssue{"POSSIBLE_DUPLICATE", fmt.Sprintf("%s (#%d) and %s (#%d) may refer to the same %s in %s", earlier.code, earlier.id, code, id, strings.ToLower(kind), building)})
		} else {
			seen[key] = found{id, code, name, building}
		}
	}
	return issues, rows.Err()
}
