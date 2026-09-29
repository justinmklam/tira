package validator

import "strings"

// DefaultIssueType returns the issue type to pre-fill when the caller has not
// chosen one: configured when it names a valid type (case-insensitively,
// normalised to the valid list's casing), otherwise valid[0]. When valid is
// empty, configured is returned as-is. configuredRejected reports that
// configured was non-empty but not accepted, so callers may warn; the returned
// type is still usable in that case.
func DefaultIssueType(configured string, valid []string) (resolved string, configuredRejected bool) {
	if configured != "" {
		for _, t := range valid {
			if strings.EqualFold(t, configured) {
				return t, false
			}
		}
		if len(valid) > 0 {
			return valid[0], true
		}
		return configured, false
	}
	if len(valid) > 0 {
		return valid[0], false
	}
	return "", false
}
