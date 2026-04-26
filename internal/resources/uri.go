// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import "strings"

// matchURI parses a concrete URI against a template and returns the captured
// segments keyed by name. Template segments use `{name}` placeholders;
// literal segments must match exactly. Returns ok=false when the URI shape
// doesn't fit the template.
//
// matchURI("librarium://library/abc/series/xyz",
//          "librarium://library/{lib}/series/{sid}")
//   → {"lib": "abc", "sid": "xyz"}, true
func matchURI(uri, template string) (map[string]string, bool) {
	uParts := strings.Split(uri, "/")
	tParts := strings.Split(template, "/")
	if len(uParts) != len(tParts) {
		return nil, false
	}
	out := map[string]string{}
	for i, t := range tParts {
		if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
			out[t[1:len(t)-1]] = uParts[i]
			continue
		}
		if t != uParts[i] {
			return nil, false
		}
	}
	return out, true
}
