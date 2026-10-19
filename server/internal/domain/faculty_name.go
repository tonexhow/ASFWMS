package domain

import (
	"errors"
	"net/mail"
	"strings"
)

// Structured edits compose a display name. Legacy rows are deliberately NOT
// split heuristically: multi-word first/surnames and titles must stay intact.
func normalizeFacultyName(input, existing map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range input {
		out[k] = v
	}
	structured := false
	for _, k := range []string{"prefix", "first_name", "middle_name", "last_name", "extension"} {
		if _, ok := input[k]; ok {
			structured = true
		}
	}
	if structured {
		values := map[string]string{}
		for _, k := range []string{"prefix", "first_name", "middle_name", "last_name", "extension"} {
			v, ok := input[k]
			if !ok {
				v = existing[k]
			}
			s, _ := v.(string)
			values[k] = strings.TrimSpace(s)
		}
		// Existing legacy names remain valid on non-name edits; a new structured name
		// requires first + last together. A blank middle name is legitimate.
		if values["first_name"] == "" || values["last_name"] == "" {
			return nil, errors.New("first name and last name are required; legacy names are preserved until you enter both")
		}
		pieces := []string{}
		for _, k := range []string{"prefix", "first_name", "middle_name", "last_name", "extension"} {
			if values[k] != "" {
				pieces = append(pieces, values[k])
			}
		}
		out["name"] = strings.Join(pieces, " ")
	}
	if v, ok := input["email"]; ok {
		email, valid := v.(string)
		if !valid {
			return nil, errors.New("email must be text")
		}
		email = strings.TrimSpace(email)
		if email != "" {
			address, err := mail.ParseAddress(email)
			if err != nil || address.Address != email || strings.ContainsAny(email, "\r\n") {
				return nil, errors.New("enter a valid email address or leave it blank")
			}
		}
		out["email"] = email
	}
	return out, nil
}
