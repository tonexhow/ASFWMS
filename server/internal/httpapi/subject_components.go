package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

type componentInput struct {
	Kind  string  `json:"kind"`
	Units float64 `json:"units"`
}
type subjectPlan struct {
	Components []componentInput
	RoomIDs    []int
	HasRooms   bool
}

func parseSubjectPlan(in map[string]any) (*subjectPlan, error) {
	raw, has := in["components"]
	roomRaw, hasRooms := in["requested_facility_ids"]
	if !has && !hasRooms {
		return nil, nil
	}
	p := &subjectPlan{HasRooms: hasRooms}
	delete(in, "components")
	delete(in, "requested_facility_ids")
	if has {
		b, _ := json.Marshal(raw)
		var entries []componentInput
		if json.Unmarshal(b, &entries) != nil || len(entries) == 0 || len(entries) > 30 {
			return nil, errors.New("add at least one unit component (maximum 30 rows)")
		}
		order := []string{}
		sums := map[string]float64{}
		total := 0.0
		for _, x := range entries {
			if x.Kind != "lecture" && x.Kind != "laboratory" && x.Kind != "field" {
				return nil, errors.New("unit type must be lecture, laboratory or field")
			}
			minutes := x.Units * 60
			if x.Units <= 0 || x.Units > 60 || math.Abs(minutes/15-math.Round(minutes/15)) > 0.00001 {
				return nil, errors.New("units must be positive quarter-unit increments, such as 1, 2 or 3")
			}
			if sums[x.Kind] == 0 {
				order = append(order, x.Kind)
			}
			sums[x.Kind] += x.Units
			total += x.Units
		}
		if total > 60 {
			return nil, errors.New("total units cannot exceed 60")
		}
		for _, kind := range order {
			p.Components = append(p.Components, componentInput{kind, sums[kind]})
		}
	}
	if hasRooms {
		b, _ := json.Marshal(roomRaw)
		if json.Unmarshal(b, &p.RoomIDs) != nil || len(p.RoomIDs) > 1000 {
			return nil, errors.New("send requested facilities as an array of room IDs")
		}
		seen := map[int]bool{}
		ids := []int{}
		for _, id := range p.RoomIDs {
			if id < 1 {
				return nil, errors.New("choose an existing facility")
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		p.RoomIDs = ids
		sort.Ints(p.RoomIDs)
	}
	return p, nil
}
func componentRoomKind(kind string) string {
	if kind == "lecture" {
		return "classroom"
	}
	return kind
}
func applySubjectTotals(in map[string]any, components []componentInput) {
	total := 0.0
	for _, c := range components {
		total += c.Units
	}
	in["units"] = total
	in["weekly_minutes"] = int(math.Round(total * 60))
	in["required_room"] = "any"
	if len(components) == 1 {
		in["required_room"] = componentRoomKind(components[0].Kind)
	}
}
func saveSubjectPlan(ctx context.Context, q storage.Querier, id int, p *subjectPlan, s domain.Snapshot) error {
	old := []domain.SubjectComponent{}
	for _, c := range s.Components {
		if c.SubjectID == id {
			old = append(old, c)
		}
	}
	changed := false
	if p.Components != nil {
		if len(old) != len(p.Components) {
			changed = true
		}
		for _, c := range p.Components {
			found := false
			for _, o := range old {
				if o.Kind == c.Kind && o.Units == c.Units && o.WeeklyMinutes == int(math.Round(c.Units*60)) {
					found = true
				}
			}
			if !found {
				changed = true
			}
		}
	}
	oldIDs := []int{}
	for _, x := range s.FacilityRequests {
		if x.SubjectID == id {
			oldIDs = append(oldIDs, x.RoomID)
		}
	}
	sort.Ints(oldIDs)
	if p.HasRooms && fmt.Sprint(oldIDs) != fmt.Sprint(p.RoomIDs) {
		changed = true
	}
	if changed {
		for _, m := range s.Meetings {
			if m.SubjectID == id {
				return errors.New("this subject has committed meetings; release its affected schedules before changing unit components or requested facilities")
			}
		}
	}
	kinds := map[string]bool{}
	if p.Components != nil {
		for _, c := range p.Components {
			kinds[componentRoomKind(c.Kind)] = true
		}
	} else {
		for _, c := range old {
			kinds[componentRoomKind(c.Kind)] = true
		}
		if len(kinds) == 0 {
			for _, su := range s.Subjects {
				if su.ID == id {
					kinds[su.RequiredRoom] = true
				}
			}
		}
	}
	requestedIDs := oldIDs
	if p.HasRooms {
		requestedIDs = p.RoomIDs
	}
	if p.HasRooms || p.Components != nil {
		for _, roomID := range requestedIDs {
			found := false
			for _, room := range s.Rooms {
				if room.ID == roomID && room.Active == 1 {
					building := getBuilding(s, room.BuildingID)
					if building.Active != 1 {
						return errors.New("requested facilities must belong to an active building")
					}
					if !kinds[room.Kind] && !kinds["any"] {
						return fmt.Errorf("%s does not match any unit component type", room.Code)
					}
					found = true
					break
				}
			}
			if !found {
				return errors.New("choose active, existing requested facilities")
			}
		}
	}
	if p.Components != nil && changed {
		keep := map[int]bool{}
		for order, c := range p.Components {
			existingID := 0
			for _, o := range old {
				if o.Kind == c.Kind && !keep[o.ID] {
					existingID = o.ID
					break
				}
			}
			minutes := int(math.Round(c.Units * 60))
			if existingID > 0 {
				keep[existingID] = true
				if _, e := q.ExecContext(ctx, "UPDATE subject_components SET units=?,weekly_minutes=?,sort_order=? WHERE id=?", c.Units, minutes, order, existingID); e != nil {
					return e
				}
			} else {
				if _, e := q.ExecContext(ctx, "INSERT INTO subject_components(subject_id,kind,units,weekly_minutes,sort_order) VALUES(?,?,?,?,?)", id, c.Kind, c.Units, minutes, order); e != nil {
					return e
				}
			}
		}
		for _, o := range old {
			if !keep[o.ID] {
				if _, e := q.ExecContext(ctx, "DELETE FROM subject_components WHERE id=?", o.ID); e != nil {
					return e
				}
			}
		}
	}
	if p.HasRooms && changed {
		if _, e := q.ExecContext(ctx, "DELETE FROM subject_facility_requests WHERE subject_id=?", id); e != nil {
			return e
		}
		for _, roomID := range p.RoomIDs {
			if _, e := q.ExecContext(ctx, "INSERT INTO subject_facility_requests(subject_id,room_id) VALUES(?,?)", id, roomID); e != nil {
				return e
			}
		}
	}
	return nil
}
func getBuilding(s domain.Snapshot, id int) domain.Building {
	for _, b := range s.Buildings {
		if b.ID == id {
			return b
		}
	}
	return domain.Building{}
}
