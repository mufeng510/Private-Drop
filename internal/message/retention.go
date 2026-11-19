package message

import "time"

// ExpiryFrom computes the expiry time for a message created at `created`
// with a retention period in days. A non-positive retention means "never".
func ExpiryFrom(created time.Time, retentionDays int) time.Time {
	if retentionDays <= 0 {
		return time.Time{}
	}
	return created.Add(time.Duration(retentionDays) * 24 * time.Hour)
}

// ExpiresWithin reports whether a message expires within the next `days`.
func ExpiresWithin(expires string, days int) bool {
	if expires == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	return t.After(now) && t.Before(now.Add(time.Duration(days)*24*time.Hour))
}
