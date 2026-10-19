package httpapi

import (
	"asfwms.local/scheduler/internal/domain"
	"asfwms.local/scheduler/internal/storage"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Linked deletion is explicitly opt-in. Only scheduling/business tables participate;
// authentication, sessions, tokens, audit and application settings are never purged.
var dependencyTables = []string{
	"departments", "programs", "academic_periods", "subjects", "curricula", "curriculum_subject_years",
	"faculty", "sections", "faculty_subjects", "teaching_assignments", "faculty_work_times",
	"faculty_considerations", "buildings", "rooms", "generation_runs", "scheduled_meetings", "users",
}

// Link rows contain only associations: unlinking one may NEVER erase its
// original subject, faculty, section or curriculum. Dependencies must be
// resolved first; unlink is not a disguised cascading deletion.
var unlinkableLinks = map[string]bool{
	"curriculum_subject_years": true,
	"faculty_subjects":         true,
	"teaching_assignments":     true,
}
var deletionOrder = []string{
	"scheduled_meetings", "teaching_assignments", "faculty_subjects", "faculty_work_times",
	"faculty_considerations", "curriculum_subject_years", "sections", "generation_runs",
	"rooms", "buildings", "faculty", "curricula", "subjects", "academic_periods", "programs", "departments",
}

type deleteItem struct {
	Table string `json:"table"`
	ID    int    `json:"id"`
	Label string `json:"label"`
	Path  string `json:"path"`
}
type deleteImpact struct {
	Table   string       `json:"table"`
	Count   int          `json:"count"`
	Samples []deleteItem `json:"samples"`
}
type detachImpact struct {
	Table   string       `json:"table"`
	Field   string       `json:"field"`
	Count   int          `json:"count"`
	Samples []deleteItem `json:"samples"`
}
type deletePreview struct {
	Table         string         `json:"table"`
	ID            int            `json:"id"`
	Label         string         `json:"label"`
	Revision      int64          `json:"revision"`
	Fingerprint   string         `json:"fingerprint"`
	Impacts       []deleteImpact `json:"impacts"`
	Detaches      []detachImpact `json:"detaches"`
	UnlinkEffects []deleteImpact `json:"unlink_effects,omitempty"`
	Total         int            `json:"total"`
	Related       int            `json:"related"`
	Blocked       bool           `json:"blocked"`
	Warnings      []string       `json:"warnings"`
	CanDelete     bool           `json:"can_delete"`
}
type deletePlan struct {
	Preview  deletePreview
	entries  map[string]map[int]bool
	detaches map[string]map[int]bool
}

func appendUnique(items []string, item string) []string {
	for _, s := range items {
		if s == item {
			return items
		}
	}
	return append(items, item)
}
func tableAllowed(table string) bool { return table == "users" || devTables[table] }
func deletePath(ctx context.Context, q storage.Querier, table string, id int) string {
	if table == "curriculum_subject_years" {
		var curriculumID int
		if q.QueryRowContext(ctx, "SELECT curriculum_id FROM curriculum_subject_years WHERE id=?", id).Scan(&curriculumID) == nil {
			return "#academic?tab=curricula&focus=" + strconv.Itoa(curriculumID)
		}
		return "#academic?tab=curricula"
	}
	switch table {
	case "departments", "programs", "subjects", "curricula", "academic_periods", "sections":
		return "#academic?tab=" + table + "&focus=" + strconv.Itoa(id)
	case "faculty":
		return "#faculty?profile=" + strconv.Itoa(id)
	case "faculty_subjects", "faculty_work_times", "faculty_considerations", "teaching_assignments":
		var facultyID int
		if q.QueryRowContext(ctx, "SELECT faculty_id FROM "+table+" WHERE id=?", id).Scan(&facultyID) == nil {
			return "#faculty?profile=" + strconv.Itoa(facultyID)
		}
		return "#faculty"
	case "buildings", "rooms":
		return "#facilities?tab=" + table + "&focus=" + strconv.Itoa(id)
	case "generation_runs", "scheduled_meetings":
		return "#schedules"
	default:
		return ""
	}
}

var legacyCurriculumLabel = regexp.MustCompile(`(?i)^(.+?)\s*/\s*Scanned loading\s+(\d{4})\s*-\s*(.+)$`)

func displayCurriculumLabel(name string) string {
	groups := legacyCurriculumLabel.FindStringSubmatch(strings.TrimSpace(name))
	if len(groups) != 4 {
		return name
	}
	program, year, group := strings.TrimSpace(groups[1]), groups[2], strings.TrimSpace(groups[3])
	if len(group) >= len(program) && strings.EqualFold(group[:len(program)], program) {
		group = strings.TrimSpace(group[len(program):])
	}
	if group == "" {
		group = strings.TrimSpace(groups[3])
	}
	return program + " / " + group + " (" + year + ")"
}
func deleteLabel(ctx context.Context, q storage.Querier, table string, id int) string {
	if table == "users" {
		return fmt.Sprintf("Account #%d (cannot be deleted here)", id)
	}
	if table == "curriculum_subject_years" {
		var subject string
		var year int
		if q.QueryRowContext(ctx, `SELECT c.year_level,s.code FROM curriculum_subject_years c JOIN subjects s ON s.id=c.subject_id WHERE c.id=?`, id).Scan(&year, &subject) == nil {
			return fmt.Sprintf("%s · %s", domain.YearLabel(year), subject)
		}
	}
	if table == "teaching_assignments" {
		var section, subject, instructor string
		if q.QueryRowContext(ctx, `SELECT sec.code, sub.code, f.name FROM teaching_assignments a JOIN sections sec ON sec.id=a.section_id JOIN subjects sub ON sub.id=a.subject_id JOIN faculty f ON f.id=a.faculty_id WHERE a.id=?`, id).Scan(&section, &subject, &instructor) == nil {
			return section + " · " + subject + " / " + instructor
		}
	}
	if table == "generation_runs" {
		var department, period string
		if q.QueryRowContext(ctx, `SELECT d.code,p.label FROM generation_runs r JOIN departments d ON d.id=r.department_id JOIN academic_periods p ON p.id=r.period_id WHERE r.id=?`, id).Scan(&department, &period) == nil {
			return fmt.Sprintf("Run #%d · %s / %s", id, department, period)
		}
	}
	lines, err := storage.ReadRows(ctx, q, "SELECT * FROM "+table+" WHERE id=?", id)
	if err != nil || len(lines) == 0 {
		return fmt.Sprintf("#%d", id)
	}
	record := lines[0]
	for _, field := range []string{"code", "label", "name", "title", "username"} {
		if val, ok := record[field].(string); ok && strings.TrimSpace(val) != "" {
			if table == "curricula" {
				return displayCurriculumLabel(val)
			}
			return val
		}
	}
	return fmt.Sprintf("#%d", id)
}

type dependencyFK struct {
	Child, Field string
	Nullable     bool
}

func foreignReferences(ctx context.Context, q storage.Querier) (map[string][]dependencyFK, error) {
	refs := map[string][]dependencyFK{}
	for _, child := range dependencyTables {
		links, err := storage.ReadRows(ctx, q, "PRAGMA foreign_key_list("+child+")")
		if err != nil {
			return nil, err
		}
		groups := map[int][]map[string]any{}
		for _, x := range links {
			groups[domain.Int(x, "id")] = append(groups[domain.Int(x, "id")], x)
		}
		cols, err := storage.ReadRows(ctx, q, "PRAGMA table_info("+child+")")
		if err != nil {
			return nil, err
		}
		nullable := map[string]bool{}
		for _, col := range cols {
			nullable[fmt.Sprint(col["name"])] = domain.Int(col, "notnull") == 0
		}
		for _, group := range groups {
			if len(group) != 1 {
				continue
			} // Composite subject eligibility links are covered separately.
			x := group[0]
			parent := fmt.Sprint(x["table"])
			to := fmt.Sprint(x["to"])
			from := fmt.Sprint(x["from"])
			if to != "id" || !tableAllowed(parent) {
				continue
			}
			refs[parent] = append(refs[parent], dependencyFK{child, from, nullable[from]})
		}
	}
	return refs, nil
}
func (p *deletePlan) add(table string, id int, queue *[]deleteItem) {
	if id <= 0 {
		return
	}
	if p.entries[table] == nil {
		p.entries[table] = map[int]bool{}
	}
	if p.entries[table][id] {
		return
	}
	p.entries[table][id] = true
	*queue = append(*queue, deleteItem{Table: table, ID: id})
}
func (p *deletePlan) detach(table, field string, id int) {
	key := table + "." + field
	if p.detaches[key] == nil {
		p.detaches[key] = map[int]bool{}
	}
	p.detaches[key][id] = true
}

// Preview JSON is a saved schedule plan, not a relational foreign key. When
// deleting a section/subject/faculty/room, invalidate preview runs that refer
// to it, rather than leaving the application with misleading stale previews.
func previewSnapshotReferences(ctx context.Context, q storage.Querier) (map[string]map[int][]int, error) {
	rows, err := storage.ReadRows(ctx, q, "SELECT id,result_json FROM generation_runs WHERE status='preview'")
	if err != nil {
		return nil, err
	}
	out := map[string]map[int][]int{}
	type reference struct {
		SectionID int `json:"section_id"`
		SubjectID int `json:"subject_id"`
		FacultyID int `json:"faculty_id"`
		RoomID    int `json:"room_id"`
	}
	for _, row := range rows {
		id := domain.Int(row, "id")
		var snapshot struct {
			Meetings []reference `json:"meetings"`
			Issues   []reference `json:"issues"`
		}
		if err := json.Unmarshal([]byte(fmt.Sprint(row["result_json"])), &snapshot); err != nil {
			return nil, fmt.Errorf("generation preview #%d has invalid saved data; remove this run in Developer Database before deleting related academic records: %w", id, err)
		}
		seen := map[string]map[int]bool{}
		for _, item := range append(snapshot.Meetings, snapshot.Issues...) {
			for _, key := range []struct {
				table string
				id    int
			}{
				{"sections", item.SectionID}, {"subjects", item.SubjectID}, {"faculty", item.FacultyID}, {"rooms", item.RoomID},
			} {
				if key.id < 1 {
					continue
				}
				if seen[key.table] == nil {
					seen[key.table] = map[int]bool{}
				}
				if seen[key.table][key.id] {
					continue
				}
				seen[key.table][key.id] = true
				if out[key.table] == nil {
					out[key.table] = map[int][]int{}
				}
				out[key.table][key.id] = append(out[key.table][key.id], id)
			}
		}
	}
	return out, nil
}

func queryIDs(ctx context.Context, q storage.Querier, sqlText string, args ...any) ([]int, error) {
	rows, err := q.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func buildDeletePlan(ctx context.Context, q storage.Querier, table string, id int) (*deletePlan, error) {
	return buildDeletePlanForIDs(ctx, q, table, []int{id})
}

// A bulk plan walks the *union* of dependencies once. Unlike invoking single-row
// deletion N times, this is a single transaction with one complete preview:
// children shared by multiple roots are counted/deleted exactly once.
func buildDeletePlanForIDs(ctx context.Context, q storage.Querier, table string, roots []int) (*deletePlan, error) {
	if !devTables[table] {
		return nil, fmt.Errorf("table is not available for deletion")
	}
	if len(roots) == 0 || len(roots) > 5000 {
		return nil, fmt.Errorf("select between 1 and 5000 records")
	}
	ids := make([]int, 0, len(roots))
	unique := map[int]bool{}
	for _, id := range roots {
		if id < 1 || unique[id] {
			return nil, fmt.Errorf("invalid or duplicate record selection")
		}
		unique[id] = true
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		found, err := queryIDs(ctx, q, "SELECT id FROM "+table+" WHERE id=?", id)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("selected record #%d no longer exists; refresh selection", id)
		}
	}
	rev, err := storage.Revision(ctx, q)
	if err != nil {
		return nil, err
	}
	refs, err := foreignReferences(ctx, q)
	if err != nil {
		return nil, err
	}
	previewRefs, err := previewSnapshotReferences(ctx, q)
	if err != nil {
		return nil, err
	}
	p := &deletePlan{Preview: deletePreview{Table: table, ID: ids[0], Revision: rev, Label: deleteLabel(ctx, q, table, ids[0])}, entries: map[string]map[int]bool{}, detaches: map[string]map[int]bool{}}
	queue := []deleteItem{}
	if len(ids) > 1 {
		p.Preview.ID = 0
		p.Preview.Label = fmt.Sprintf("%d selected %s", len(ids), table)
	}
	for _, rootID := range ids {
		p.add(table, rootID, &queue)
	}
	for i := 0; i < len(queue); i++ {
		item := queue[i]
		for _, previewID := range previewRefs[item.Table][item.ID] {
			p.add("generation_runs", previewID, &queue)
		}
		if len(queue) > 50000 {
			return nil, fmt.Errorf("too many linked records; narrow the deletion scope")
		}
		for _, fk := range refs[item.Table] {
			targets, err := queryIDs(ctx, q, "SELECT id FROM "+fk.Child+" WHERE "+fk.Field+"=? ORDER BY id", item.ID)
			if err != nil {
				return nil, err
			}
			if fk.Nullable && fk.Child != "users" {
				for _, childID := range targets {
					p.detach(fk.Child, fk.Field, childID)
					// An orphaned section must not retain a preview or committed
					// timetable generated under its now-deleted academic context.
					if fk.Child == "sections" && (fk.Field == "curriculum_id" || fk.Field == "period_id" || fk.Field == "program_id") {
						for _, runID := range previewRefs["sections"][childID] {
							p.add("generation_runs", runID, &queue)
						}
						runIDs, e := queryIDs(ctx, q, "SELECT DISTINCT run_id FROM scheduled_meetings WHERE section_id=?", childID)
						if e != nil {
							return nil, e
						}
						for _, runID := range runIDs {
							p.add("generation_runs", runID, &queue)
						}
					}
				}
				continue
			}
			for _, childID := range targets {
				p.add(fk.Child, childID, &queue)
			}
		}
		// A teaching assignment also references faculty_subjects by composite key.
		if item.Table == "faculty_subjects" {
			pairs, err := storage.ReadRows(ctx, q, "SELECT faculty_id,subject_id FROM faculty_subjects WHERE id=?", item.ID)
			if err != nil {
				return nil, err
			}
			if len(pairs) > 0 {
				ids, e := queryIDs(ctx, q, "SELECT id FROM teaching_assignments WHERE faculty_id=? AND subject_id=?", domain.Int(pairs[0], "faculty_id"), domain.Int(pairs[0], "subject_id"))
				if e != nil {
					return nil, e
				}
				for _, linkedID := range ids {
					p.add("teaching_assignments", linkedID, &queue)
				}
			}
		}
		// Curriculum membership can support an instructor link without an SQL FK;
		// deleting it must also resolve those semantic links.
		if item.Table == "curriculum_subject_years" {
			info, e := storage.ReadRows(ctx, q, "SELECT curriculum_id,year_level FROM curriculum_subject_years WHERE id=?", item.ID)
			if e != nil {
				return nil, e
			}
			if len(info) > 0 {
				affected, e := queryIDs(ctx, q, "SELECT id FROM sections WHERE curriculum_id=? AND year_level=?", domain.Int(info[0], "curriculum_id"), domain.Int(info[0], "year_level"))
				if e != nil {
					return nil, e
				}
				for _, sid := range affected {
					for _, runID := range previewRefs["sections"][sid] {
						p.add("generation_runs", runID, &queue)
					}
				}
			}
			ids, e := queryIDs(ctx, q, `SELECT a.id FROM teaching_assignments a JOIN sections s ON s.id=a.section_id JOIN curriculum_subject_years c ON c.curriculum_id=s.curriculum_id AND c.year_level=s.year_level AND c.subject_id=a.subject_id WHERE c.id=?`, item.ID)
			if e != nil {
				return nil, e
			}
			for _, linkedID := range ids {
				p.add("teaching_assignments", linkedID, &queue)
			}
		}
		if item.Table == "teaching_assignments" {
			pairs, e := storage.ReadRows(ctx, q, "SELECT faculty_id,subject_id,section_id FROM teaching_assignments WHERE id=?", item.ID)
			if e != nil {
				return nil, e
			}
			if len(pairs) > 0 {
				m := pairs[0]
				runIDs, e := queryIDs(ctx, q, "SELECT DISTINCT run_id FROM scheduled_meetings WHERE faculty_id=? AND subject_id=? AND section_id=?", domain.Int(m, "faculty_id"), domain.Int(m, "subject_id"), domain.Int(m, "section_id"))
				if e != nil {
					return nil, e
				}
				for _, runID := range runIDs {
					p.add("generation_runs", runID, &queue)
				}
			}
		}
		if item.Table == "faculty_subjects" {
			pairs, e := storage.ReadRows(ctx, q, "SELECT faculty_id,subject_id FROM faculty_subjects WHERE id=?", item.ID)
			if e != nil {
				return nil, e
			}
			if len(pairs) > 0 {
				m := pairs[0]
				runIDs, e := queryIDs(ctx, q, "SELECT DISTINCT run_id FROM scheduled_meetings WHERE faculty_id=? AND subject_id=?", domain.Int(m, "faculty_id"), domain.Int(m, "subject_id"))
				if e != nil {
					return nil, e
				}
				for _, runID := range runIDs {
					p.add("generation_runs", runID, &queue)
				}
			}
		}
		if item.Table == "curriculum_subject_years" {
			runIDs, e := queryIDs(ctx, q, `SELECT DISTINCT m.run_id FROM scheduled_meetings m JOIN sections s ON s.id=m.section_id JOIN curriculum_subject_years c ON c.curriculum_id=s.curriculum_id AND c.year_level=s.year_level AND c.subject_id=m.subject_id WHERE c.id=?`, item.ID)
			if e != nil {
				return nil, e
			}
			for _, runID := range runIDs {
				p.add("generation_runs", runID, &queue)
			}
		}
		// A generation run is an indivisible saved timetable. Removing a class
		// releases the entire affected run, rather than leaving a partial/stale run.
		if item.Table == "scheduled_meetings" {
			runIDs, e := queryIDs(ctx, q, "SELECT run_id FROM scheduled_meetings WHERE id=?", item.ID)
			if e != nil {
				return nil, e
			}
			for _, runID := range runIDs {
				p.add("generation_runs", runID, &queue)
			}
		}
	}
	// If the parent is itself deleted, no SET NULL is needed for that row.
	for key, ids := range p.detaches {
		t := strings.SplitN(key, ".", 2)[0]
		for id := range ids {
			if p.entries[t][id] {
				delete(ids, id)
			}
		}
	}
	groups := []deleteImpact{}
	detached := []detachImpact{}
	keys := make([]string, 0, len(p.entries))
	for name := range p.entries {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		ids := p.entries[name]
		idList := make([]int, 0, len(ids))
		for n := range ids {
			idList = append(idList, n)
		}
		sort.Ints(idList)
		g := deleteImpact{Table: name, Count: len(ids), Samples: []deleteItem{}}
		for _, n := range idList {
			if len(g.Samples) >= 6 {
				break
			}
			g.Samples = append(g.Samples, deleteItem{name, n, deleteLabel(ctx, q, name, n), deletePath(ctx, q, name, n)})
		}
		groups = append(groups, g)
		p.Preview.Total += len(ids)
		if name != "users" && name != table {
			p.Preview.Related += len(ids)
		}
		if name == "users" {
			p.Preview.Blocked = true
			p.Preview.Warnings = appendUnique(p.Preview.Warnings, "Department has linked user accounts. Transfer or disable their department scope before deleting; authentication data will never be deleted by this tool.")
		}
	}
	detKeys := make([]string, 0, len(p.detaches))
	for key := range p.detaches {
		detKeys = append(detKeys, key)
	}
	sort.Strings(detKeys)
	for _, key := range detKeys {
		ids := p.detaches[key]
		if len(ids) == 0 {
			continue
		}
		parts := strings.SplitN(key, ".", 2)
		d := detachImpact{Table: parts[0], Field: parts[1], Count: len(ids), Samples: []deleteItem{}}
		idList := make([]int, 0, len(ids))
		for n := range ids {
			idList = append(idList, n)
		}
		sort.Ints(idList)
		for _, n := range idList {
			if len(d.Samples) >= 6 {
				break
			}
			d.Samples = append(d.Samples, deleteItem{parts[0], n, deleteLabel(ctx, q, parts[0], n), deletePath(ctx, q, parts[0], n)})
		}
		detached = append(detached, d)
		p.Preview.Related += len(ids)
	}
	p.Preview.Detaches = detached
	p.Preview.Impacts = groups
	// A curriculum membership is a link, not a second subject row. Removing a
	// membership changes which subjects its sections require even when no
	// explicit instructor has been assigned yet. Warn about that non-destructive
	// consequence without incorrectly classifying sections as deleted children.
	if table == "curriculum_subject_years" {
		affected := map[int]bool{}
		for _, rootID := range ids {
			membership, e := storage.ReadRows(ctx, q, "SELECT curriculum_id,year_level FROM curriculum_subject_years WHERE id=?", rootID)
			if e != nil {
				return nil, e
			}
			if len(membership) == 0 {
				continue
			}
			curriculumID, year := domain.Int(membership[0], "curriculum_id"), domain.Int(membership[0], "year_level")
			sectionIDs, e := queryIDs(ctx, q, "SELECT id FROM sections WHERE curriculum_id=? AND year_level=? ORDER BY id", curriculumID, year)
			if e != nil {
				return nil, e
			}
			for _, sectionID := range sectionIDs {
				affected[sectionID] = true
			}
		}
		if len(affected) > 0 {
			ids := make([]int, 0, len(affected))
			for sectionID := range affected {
				ids = append(ids, sectionID)
			}
			sort.Ints(ids)
			effect := deleteImpact{Table: "sections", Count: len(ids), Samples: []deleteItem{}}
			for _, sectionID := range ids {
				if len(effect.Samples) == 6 {
					break
				}
				effect.Samples = append(effect.Samples, deleteItem{Table: "sections", ID: sectionID, Label: deleteLabel(ctx, q, "sections", sectionID), Path: deletePath(ctx, q, "sections", sectionID)})
			}
			p.Preview.UnlinkEffects = append(p.Preview.UnlinkEffects, effect)
			p.Preview.Warnings = appendUnique(p.Preview.Warnings, fmt.Sprintf("Unlinking selected subject-year memberships will change the required subjects of %d section(s). Those section and subject records remain.", len(ids)))
		}
	}

	for _, s := range []struct{ table, msg string }{
		{"sections", "Linked sections will be deleted, along with their adviser references and instructor assignments."},
		{"generation_runs", "Saved timetable runs will be removed; their meetings and reservations will be released."},
		{"scheduled_meetings", "Committed class meetings will be removed. Other departments may then reuse released rooms."},
		{"curriculum_subject_years", "Subject-year memberships will be removed from the selected curriculum."},
	} {
		if len(p.entries[s.table]) > 0 && (s.table != table || len(p.entries[s.table]) > 1) {
			p.Preview.Warnings = appendUnique(p.Preview.Warnings, s.msg)
		}
	}
	// Removing a parent does not delete independent rows such as sections,
	// classrooms or curricula; their parent reference is cleared to NULL.
	for _, link := range p.Preview.Detaches {
		if link.Field == "curriculum_id" && link.Table == "sections" {
			p.Preview.Warnings = appendUnique(p.Preview.Warnings, fmt.Sprintf("%d section(s) will stay in the database with Curriculum: Unassigned. Re-link a curriculum before scheduling them.", link.Count))
		}
		if link.Field == "period_id" && link.Table == "sections" {
			p.Preview.Warnings = appendUnique(p.Preview.Warnings, fmt.Sprintf("%d section(s) will stay in the database with Academic period: Unassigned. Re-link a period before scheduling them.", link.Count))
		}
		if link.Field == "building_id" && link.Table == "rooms" {
			p.Preview.Warnings = appendUnique(p.Preview.Warnings, fmt.Sprintf("%d room(s) will remain but will have no building. Reassign them from Facilities.", link.Count))
		}
	}
	if len(p.Preview.Detaches) > 0 {
		p.Preview.Warnings = appendUnique(p.Preview.Warnings, "Optional references (such as a section adviser) will be cleared instead of deleting the linked record.")
	}
	p.Preview.CanDelete = !p.Preview.Blocked
	hash := sha256.New()
	for _, g := range groups {
		fmt.Fprintf(hash, "%s:", g.Table)
		ids := p.entries[g.Table]
		sorted := make([]int, 0, len(ids))
		for i := range ids {
			sorted = append(sorted, i)
		}
		sort.Ints(sorted)
		for _, i := range sorted {
			fmt.Fprintf(hash, "%d,", i)
		}
	}
	for _, g := range detached {
		fmt.Fprintf(hash, "detach:%s.%s:", g.Table, g.Field)
		ids := p.detaches[g.Table+"."+g.Field]
		sorted := make([]int, 0, len(ids))
		for i := range ids {
			sorted = append(sorted, i)
		}
		sort.Ints(sorted)
		for _, i := range sorted {
			fmt.Fprintf(hash, "%d,", i)
		}
	}
	p.Preview.Fingerprint = hex.EncodeToString(hash.Sum(nil))
	return p, nil
}
func (a *App) recordDependencies(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("kind")
	id, e := idOf(r)
	if e != nil {
		problem(w, 400, "ID", e.Error())
		return
	}
	p, e := buildDeletePlan(r.Context(), a.Store.DB, table, id)
	if e != nil {
		problem(w, 404, "DEPENDENCIES", safeError(e))
		return
	}
	// A department Scheduler may review an unlinked record they own, but
	// must not enumerate protected/global or other-department dependencies.
	if user(r).Role == "scheduler" && p.Preview.Related > 0 {
		p.Preview.Impacts = nil
		p.Preview.Detaches = nil
		p.Preview.CanDelete = false
		p.Preview.Blocked = true
		p.Preview.Warnings = []string{"This record has linked data that requires an Administrator to review. The system did not disclose other departments' records."}
	}
	writeJSON(w, 200, p.Preview)
}
func executeDeletePlan(ctx context.Context, q storage.Querier, p *deletePlan) error {
	// Release saved timetable rows BEFORE clearing section context or unlinking
	// subject/faculty links. Database guard triggers reject those changes while
	// committed meetings exist, including when invoked directly from SQL.
	for _, table := range []string{"scheduled_meetings", "generation_runs"} {
		keys := make([]int, 0, len(p.entries[table]))
		for id := range p.entries[table] {
			keys = append(keys, id)
		}
		sort.Ints(keys)
		for _, id := range keys {
			if _, err := q.ExecContext(ctx, "DELETE FROM "+table+" WHERE id=?", id); err != nil {
				return err
			}
		}
	}
	for _, detach := range p.Preview.Detaches {
		ids := p.detaches[detach.Table+"."+detach.Field]
		keys := make([]int, 0, len(ids))
		for id := range ids {
			keys = append(keys, id)
		}
		sort.Ints(keys)
		for _, id := range keys {
			if _, err := q.ExecContext(ctx, "UPDATE "+detach.Table+" SET "+detach.Field+"=NULL WHERE id=?", id); err != nil {
				return err
			}
		}
	}
	for _, table := range deletionOrder {
		if table == "scheduled_meetings" || table == "generation_runs" {
			continue
		}
		keys := make([]int, 0, len(p.entries[table]))
		for id := range p.entries[table] {
			keys = append(keys, id)
		}
		sort.Ints(keys)
		for _, id := range keys {
			if _, err := q.ExecContext(ctx, "DELETE FROM "+table+" WHERE id=?", id); err != nil {
				return err
			}
		}
	}
	return nil
}

// A pre-delete backup is created before touching any linked records. The helper
// uses a consistent SQLite snapshot and never overwrites an existing file.
func (a *App) linkedDeleteBackup(ctx context.Context) (string, error) {
	var seq int
	var name, dbPath string
	if e := a.Store.DB.QueryRowContext(ctx, "PRAGMA database_list").Scan(&seq, &name, &dbPath); e != nil {
		return "", e
	}
	if dbPath == "" {
		return "", fmt.Errorf("cannot back up an in-memory database for linked deletion")
	}
	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "before-linked-delete-"+time.Now().UTC().Format("20060102T150405.000000000")+".db")
	if _, err := a.Store.DB.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0600)
	return path, nil
}
func (a *App) applyBusinessDeletion(ctx context.Context, table string, id int, strategy, fingerprint string, expected int64, acting User) (string, *deletePreview, error) {
	if strategy != "linked" && strategy != "preserve" && strategy != "single" && strategy != "unlink" {
		return "", nil, fmt.Errorf("choose unlink, single, or delete-and-unlink")
	}
	if strategy == "unlink" && !unlinkableLinks[table] {
		return "", nil, fmt.Errorf("only relationship records can be unlinked; delete the parent only after its links are resolved")
	}
	if (strategy == "linked" || strategy == "preserve") && !roleAdmin(acting) {
		return "", nil, fmt.Errorf("deleting records with dependencies requires Developer or Administrator")
	}
	// Avoid backing up for unauthorized or obsolete requests.
	preview, err := buildDeletePlan(ctx, a.Store.DB, table, id)
	if err != nil {
		return "", nil, err
	}
	if preview.Preview.Blocked {
		return "", &preview.Preview, fmt.Errorf("protected account references must be transferred first")
	}
	if expected >= 0 && preview.Preview.Revision != expected {
		return "", &preview.Preview, fmt.Errorf("records changed; review dependencies again")
	}
	if (strategy == "linked" || strategy == "preserve" || strategy == "unlink") && (fingerprint == "" || preview.Preview.Fingerprint != fingerprint) {
		return "", &preview.Preview, fmt.Errorf("linked records changed; review the new deletion preview")
	}
	if (strategy == "single" || strategy == "unlink") && preview.Preview.Related > 0 {
		return "", &preview.Preview, fmt.Errorf("this record has %d related records; review them and choose an explicit resolution", preview.Preview.Related)
	}
	backupName := ""
	if (strategy == "linked" || strategy == "preserve") && preview.Preview.Related > 0 {
		path, e := a.linkedDeleteBackup(ctx)
		if e != nil {
			return "", nil, fmt.Errorf("could not create a safety backup: %w", e)
		}
		backupName = filepath.Base(path)
	}
	err = a.Store.Write(ctx, func(q storage.Querier) error {
		current, e := buildDeletePlan(ctx, q, table, id)
		if e != nil {
			return e
		}
		if current.Preview.Blocked || current.Preview.Fingerprint != preview.Preview.Fingerprint || current.Preview.Revision != preview.Preview.Revision {
			return fmt.Errorf("dependency preview changed; refresh before deleting")
		}
		if (strategy == "single" || strategy == "unlink") && current.Preview.Related > 0 {
			return fmt.Errorf("linked records must be resolved first")
		}
		if e = executeDeletePlan(ctx, q, current); e != nil {
			return e
		}
		s, _, e := domain.Load(ctx, q)
		if e != nil {
			return e
		}
		if e = validateSnapshot(s); e != nil {
			return e
		}
		return audit(ctx, q, acting, "delete-"+strategy, table, id, fmt.Sprintf("deleted=%d linked=%d backup=%s", current.Preview.Total, current.Preview.Related, backupName))
	})
	if err != nil {
		return backupName, nil, err
	}
	return backupName, &preview.Preview, nil
}

// moveLinkedSections transfers sections to a compatible parent instead of
// removing them, and then deletes the obsolete academic period/curriculum.
// It deliberately releases affected timetable runs to prevent stale bookings.
func (a *App) moveLinkedSections(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "curricula" && kind != "academic_periods" {
		problem(w, 400, "MOVE", "Only curricula and academic periods support this resolution.")
		return
	}
	if !roleAdmin(user(r)) {
		deny(w)
		return
	}
	id, err := idOf(r)
	if err != nil {
		problem(w, 400, "ID", err.Error())
		return
	}
	var in struct {
		Target      int    `json:"target_id"`
		Confirm     string `json:"confirm"`
		Revision    int64  `json:"revision"`
		Fingerprint string `json:"fingerprint"`
	}
	if err = decode(r, &in); err != nil || in.Confirm != "MOVE AND DELETE" || in.Target < 1 || in.Target == id || in.Fingerprint == "" {
		problem(w, 400, "CONFIRM", "Select another parent and type MOVE AND DELETE.")
		return
	}
	prev, err := buildDeletePlan(r.Context(), a.Store.DB, kind, id)
	if err != nil {
		problem(w, 404, "MOVE", safeError(err))
		return
	}
	if prev.Preview.Revision != in.Revision || prev.Preview.Fingerprint != in.Fingerprint {
		problem(w, 409, "STALE", "Dependencies changed; refresh and review again.")
		return
	}
	if prev.Preview.Blocked {
		problem(w, 409, "PROTECTED", "Transfer linked accounts before removing their department.")
		return
	}
	if kind == "curricula" {
		var src, dest int
		e := a.Store.DB.QueryRowContext(r.Context(), "SELECT program_id FROM curricula WHERE id=?", id).Scan(&src)
		if e == nil {
			e = a.Store.DB.QueryRowContext(r.Context(), "SELECT program_id FROM curricula WHERE id=?", in.Target).Scan(&dest)
		}
		if e != nil || src != dest {
			problem(w, 409, "CURRICULUM", "Choose a destination curriculum from the same program.")
			return
		}
	}
	if kind == "academic_periods" {
		var existing int
		if a.Store.DB.QueryRowContext(r.Context(), "SELECT 1 FROM academic_periods WHERE id=?", in.Target).Scan(&existing) != nil {
			problem(w, 404, "PERIOD", "Destination academic period not found.")
			return
		}
	}
	backupPath, err := a.linkedDeleteBackup(r.Context())
	if err != nil {
		problem(w, 500, "BACKUP", "Could not create backup: "+safeError(err))
		return
	}
	moved, removedRuns := int64(0), 0
	err = a.Store.Write(r.Context(), func(q storage.Querier) error {
		current, e := buildDeletePlan(r.Context(), q, kind, id)
		if e != nil {
			return e
		}
		if current.Preview.Fingerprint != in.Fingerprint || current.Preview.Revision != in.Revision {
			return fmt.Errorf("linked records changed; review again")
		}
		if kind == "curricula" {
			// All currently required subjects for each affected year must be offered
			// by the replacement curriculum, or saved explicit assignments break.
			missing, e := storage.ReadRows(r.Context(), q, `SELECT DISTINCT c.year_level,sub.code FROM curriculum_subject_years c JOIN subjects sub ON sub.id=c.subject_id WHERE c.curriculum_id=? AND c.year_level IN (SELECT year_level FROM sections WHERE curriculum_id=?) AND NOT EXISTS(SELECT 1 FROM curriculum_subject_years dest WHERE dest.curriculum_id=? AND dest.year_level=c.year_level AND dest.subject_id=c.subject_id) LIMIT 6`, id, id, in.Target)
			if e != nil {
				return e
			}
			if len(missing) > 0 {
				items := []string{}
				for _, item := range missing {
					items = append(items, fmt.Sprintf("%s / %s", domain.YearLabel(domain.Int(item, "year_level")), item["code"]))
				}
				return fmt.Errorf("replacement curriculum is missing required subjects: %s. Add them before transferring sections", strings.Join(items, ", "))
			}
		}
		// Includes committed meetings and JSON-only generation previews that
		// mention sections in the old curriculum or academic period.
		ids := make([]int, 0, len(current.entries["generation_runs"]))
		for runID := range current.entries["generation_runs"] {
			ids = append(ids, runID)
		}
		sort.Ints(ids)
		removedRuns = len(ids)
		for _, runID := range ids {
			if _, e = q.ExecContext(r.Context(), "DELETE FROM scheduled_meetings WHERE run_id=?", runID); e != nil {
				return e
			}
			if _, e = q.ExecContext(r.Context(), "DELETE FROM generation_waits WHERE source_run_id=? OR latest_run_id=?", runID, runID); e != nil {
				return e
			}
			if _, e = q.ExecContext(r.Context(), "DELETE FROM generation_runs WHERE id=?", runID); e != nil {
				return e
			}
		}
		field := "curriculum_id"
		if kind == "academic_periods" {
			field = "period_id"
		}
		result, e := q.ExecContext(r.Context(), "UPDATE sections SET "+field+"=? WHERE "+field+"=?", in.Target, id)
		if e != nil {
			return e
		}
		moved, _ = result.RowsAffected()
		if kind == "curricula" {
			if _, e = q.ExecContext(r.Context(), "DELETE FROM curriculum_subject_years WHERE curriculum_id=?", id); e != nil {
				return e
			}
		}
		if _, e = q.ExecContext(r.Context(), "DELETE FROM "+kind+" WHERE id=?", id); e != nil {
			return e
		}
		after, _, e := domain.Load(r.Context(), q)
		if e != nil {
			return e
		}
		if e = validateSnapshot(after); e != nil {
			return e
		}
		return audit(r.Context(), q, user(r), "transfer-and-delete", kind, id, fmt.Sprintf("moved_sections=%d target=%d removed_runs=%d backup=%s", moved, in.Target, removedRuns, filepath.Base(backupPath)))
	})
	if err != nil {
		problem(w, 409, "MOVE_BLOCKED", safeError(err))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "sections_moved": moved, "runs_released": removedRuns, "backup": filepath.Base(backupPath)})
}
