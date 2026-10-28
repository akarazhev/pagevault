package archive

import "time"

// Browser captures use RFC3339 timestamps with an explicit timezone.
func validTimestamp(stamp string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return false
	}
	_, offset := parsed.Zone()
	return offset > -86400 && offset < 86400
}
