// Package schedule decides when the frame should next change photo, taking
// quiet hours into account. It is pure: all functions take the time as input.
package schedule

import (
	"time"

	"github.com/chinmay28/photoframe/internal/config"
)

// InQuiet reports whether t falls inside the quiet window [From, Until).
// A window whose From is after Until wraps past midnight.
func InQuiet(t time.Time, q config.Quiet) bool {
	if !q.Enabled || q.From == q.Until {
		return false
	}
	m := config.ClockTime(t.Hour()*60 + t.Minute())
	if q.From < q.Until {
		return m >= q.From && m < q.Until
	}
	return m >= q.From || m < q.Until
}

// QuietEnd returns the first moment at or after t that is outside the quiet
// window. If t is not in quiet hours it is returned unchanged.
func QuietEnd(t time.Time, q config.Quiet) time.Time {
	if !InQuiet(t, q) {
		return t
	}
	y, mo, d := t.Date()
	end := time.Date(y, mo, d, int(q.Until)/60, int(q.Until)%60, 0, 0, t.Location())
	if !end.After(t) {
		end = end.AddDate(0, 0, 1)
	}
	return end
}

// Next returns when the photo shown at last should be replaced.
// The result is pushed past quiet hours when it would fall inside them.
func Next(last time.Time, interval time.Duration, q config.Quiet) time.Time {
	return QuietEnd(last.Add(interval), q)
}
