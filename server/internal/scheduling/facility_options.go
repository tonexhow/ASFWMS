package scheduling

import (
	"asfwms.local/scheduler/internal/domain"
	"context"
	"errors"
)

type FacilityOption struct {
	FacilityCandidate
	Capacity  int    `json:"capacity"`
	Available bool   `json:"available"`
	Requested bool   `json:"requested"`
	Reason    string `json:"reason,omitempty"`
}

// Options checks each room against the selected unit component and the current
// preview's occupied section/faculty/room times. This is a candidate check, not
// a reservation or a guarantee that all components will fit together.
func FacilityOptions(ctx context.Context, s domain.Snapshot, result Result, sectionID, subjectID int) ([]FacilityOption, error) {
	req := result.Request
	req.Mode = "force"
	req.BorrowPolicy = "force"
	e, err := prepare(s, req)
	if err != nil {
		return nil, err
	}
	requirements, _ := e.requirements()
	var q Requirement
	for _, v := range requirements {
		if v.Section.ID == sectionID && v.Subject.ID == subjectID {
			q = v
			break
		}
	}
	if q.Section.ID == 0 {
		return nil, errors.New("section-subject is not required in this generation")
	}
	busy := append([]domain.Meeting{}, e.base...)
	for _, m := range result.Meetings {
		if m.SectionID != sectionID || m.SubjectID != subjectID {
			busy = append(busy, m)
		}
	}
	out := []FacilityOption{}
	seen := map[int]bool{}
	for _, component := range e.componentRequirements(q) {
		for _, room := range s.Rooms {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if seen[room.ID] || room.Active != 1 || e.buildings[room.BuildingID].Active != 1 || (component.Subject.RequiredRoom != "any" && room.Kind != component.Subject.RequiredRoom) {
				continue
			}
			seen[room.ID] = true
			option := FacilityOption{FacilityCandidate: FacilityCandidate{ID: room.ID, Code: room.Code, Kind: room.Kind, BuildingID: room.BuildingID, DepartmentID: room.DepartmentID, OwnerReady: e.ownerReady(room.DepartmentID), Borrowed: room.DepartmentID != 0 && room.DepartmentID != req.DepartmentID}, Capacity: room.Capacity, Requested: e.requestAllows(component, room)}
			if room.Capacity > 0 && room.Capacity < q.Section.Size {
				option.Reason = "Insufficient capacity"
				out = append(out, option)
				continue
			}
			search := *e
			search.r.FacilityChoices = []FacilityChoice{{SectionID: sectionID, SubjectID: subjectID, RoomIDs: []int{room.ID}}}
			faculties := []domain.Faculty{q.Faculty}
			if !q.Explicit {
				faculties = e.eligibleFaculty(q)
			}
			option.Reason = "No permitted time for the faculty, section and component"
			for _, faculty := range faculties {
				part := component
				part.Faculty = faculty
				ms, reason, err := search.place(ctx, part, busy)
				if err != nil {
					return nil, err
				}
				if len(ms) > 0 {
					option.Available = true
					option.Reason = ""
					break
				}
				option.Reason = failureMessage(reason, q.Explicit)
			}
			out = append(out, option)
		}
	}
	return out, nil
}
