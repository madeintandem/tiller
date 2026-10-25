package serve

import "testing"

// TestEventsLimitClamp pins the pure limit clamp directly, which the route
// tests cannot: the seven-event fixture is smaller than every bound, so an
// over-cap ask through the mux is invisible. Here the cap is pinnable.
func TestEventsLimitClamp(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", eventsDefaultLimit},    // absent
		{"abc", eventsDefaultLimit}, // unparsable
		{"0", eventsDefaultLimit},   // non-positive falls back, never zero
		{"-3", eventsDefaultLimit},  // negative falls back
		{"5", 5},                    // an in-range ask is honored
		{"1000", eventsMaxLimit},    // exactly the cap
		{"999999", eventsMaxLimit},  // over the cap is clamped, not honored
	}
	for _, c := range cases {
		if got := eventsLimit(c.raw); got != c.want {
			t.Errorf("eventsLimit(%q) = %d, want %d", c.raw, got, c.want)
		}
	}
}
