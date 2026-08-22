// window.go — when a vendor charges its peak rates.
package billing

import "time"

// PeakWindow is the vendor's own peak schedule. The rule is published in the
// vendor's timezone — DeepSeek's 09:00-12:00 and 14:00-18:00 are Beijing hours
// — so the zone travels with it and never comes from the reader's machine.
type PeakWindow struct {
	// OffsetSeconds fixes that zone. China does not observe DST, so an offset
	// is exact for these rules and costs no embedded tzdata.
	OffsetSeconds int
	// Hours are [from, to) in that zone.
	Hours [][2]int
	// WeekendOffPeak is the date a vendor stopped charging peak rates on
	// Saturdays and Sundays, YYYY-MM-DD in the same zone. Empty = no such rule.
	WeekendOffPeak string
}

// beijing is where every rate in this catalog is published.
const beijing = 8 * 3600

// IsPeak reports whether t falls inside the window. A nil window never peaks,
// which is what a vendor billing one rate around the clock means.
func (w *PeakWindow) IsPeak(t time.Time) bool {
	if w == nil || len(w.Hours) == 0 {
		return false
	}
	local := t.In(time.FixedZone("", w.OffsetSeconds))
	if w.weekendOff(local) {
		return false
	}
	hour := local.Hour()
	for _, r := range w.Hours {
		if hour >= r[0] && hour < r[1] {
			return true
		}
	}
	return false
}

// weekendOff reports whether local lands on a weekend the vendor has already
// flattened to off-peak. Compared as dates in the vendor's zone: the rule's
// start is a wall-clock midnight there, not an instant.
func (w *PeakWindow) weekendOff(local time.Time) bool {
	if w.WeekendOffPeak == "" {
		return false
	}
	if day := local.Weekday(); day != time.Saturday && day != time.Sunday {
		return false
	}
	return local.Format("2006-01-02") >= w.WeekendOffPeak
}
