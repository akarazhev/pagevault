package archive

import "testing"

func TestRFC3339TimestampValidation(t *testing.T) {
	for _, tc := range []struct {
		stamp string
		valid bool
	}{
		{"2026-10-27T12:00:00Z", true},
		{"2026-10-27T12:00:00.123456789Z", true},
		{"2026-10-27T15:00:00+03:00", true},
		{"2024-02-29T01:02:03-07:00", true},
		{"2026-10-27T12:00:00", false},
		{"2026-02-29T12:00:00Z", false},
		{"2026-10-27T24:00:00Z", false},
		{"2026-10-27T12:00:00+24:00", false},
		{"2026-W44-2T12:00:00Z", false},
		{"20261027T120000Z", false},
		{"2026-10-27T12:00:00+0300", false},
	} {
		t.Run(tc.stamp, func(t *testing.T) {
			if got := validTimestamp(tc.stamp); got != tc.valid {
				t.Fatalf("validTimestamp(%q) = %v, want %v", tc.stamp, got, tc.valid)
			}
		})
	}
}
func TestInvalidTimestampLeavesArchiveUnchanged(t *testing.T) {
	s := store(t)
	for _, stamp := range []string{"not a timestamp", "2026-10-27T12:00:00", "2026-02-29T12:00:00Z"} {
		c := capture()
		c["captured_at"] = stamp
		if _, e := s.Save(c); e == nil {
			t.Fatal("accepted invalid timestamp", stamp)
		}
	}
	stats, e := s.Stats()
	if e != nil || stats["pages"] != int64(0) {
		t.Fatal(stats, e)
	}
}
