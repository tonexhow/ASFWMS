// Package facilities identifies possible aliases for the same physical room.
// This does not change room IDs or remove historical schedule references.
package facilities

import (
	"fmt"
	"strings"
	"unicode"
)

// Key groups equal building, facility type and normalized physical name.
// A blank room name is deliberately NOT treated as a duplicate.
func Key(buildingID, roomID int, kind, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "laboratory", "lab")
	name = strings.ReplaceAll(name, "classroom", "room")
	name = strings.Join(strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), "")
	if buildingID == 0 || name == "" {
		return fmt.Sprintf("unique:%d", roomID)
	}
	return fmt.Sprintf("%d:%s:%s", buildingID, strings.ToLower(kind), name)
}
