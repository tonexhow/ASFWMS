package domain

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

// Days use the existing ISO numbering: Monday=1 ... Saturday=6, Sunday=7.
// Old named values are accepted at the API boundary for compatible local clients.
func NormalizeDays(value string) (string, error) {
	switch value {
	case "weekdays":
		value = "1,2,3,4,5"
	case "saturday":
		value = "6"
	case "sunday":
		value = "7"
	case "weekdays_saturday":
		value = "1,2,3,4,5,6"
	}
	set := map[int]bool{}
	for _, token := range strings.Split(value, ",") {
		day, err := strconv.Atoi(strings.TrimSpace(token))
		if err != nil || day < 1 || day > 7 || set[day] {
			return "", errors.New("select at least one meeting day; duplicate or invalid days are not allowed")
		}
		set[day] = true
	}
	keys := []int{}
	for day := range set {
		keys = append(keys, day)
	}
	sort.Ints(keys)
	parts := []string{}
	for _, day := range keys {
		parts = append(parts, strconv.Itoa(day))
	}
	return strings.Join(parts, ","), nil
}
func ContainsDay(value string, day int) bool {
	normalized, err := NormalizeDays(value)
	if err != nil {
		return false
	}
	for _, token := range strings.Split(normalized, ",") {
		if token == strconv.Itoa(day) {
			return true
		}
	}
	return false
}
