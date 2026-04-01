package scheduler

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// TimeSlot represents a time of day (hour + minute).
type TimeSlot struct {
	Hour   int
	Minute int
}

func (t TimeSlot) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

// ParseSchedule parses a comma-separated schedule string like "09:00,18:00".
func ParseSchedule(s string) ([]TimeSlot, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	slots := make([]TimeSlot, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		var h, m int
		if _, err := fmt.Sscanf(p, "%d:%d", &h, &m); err != nil {
			return nil, fmt.Errorf("invalid time %q: expected HH:MM", p)
		}
		if h < 0 || h > 23 || m < 0 || m > 59 {
			return nil, fmt.Errorf("invalid time %q: hour 0-23, minute 0-59", p)
		}
		slots = append(slots, TimeSlot{Hour: h, Minute: m})
	}

	// Sort chronologically.
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].Hour != slots[j].Hour {
			return slots[i].Hour < slots[j].Hour
		}
		return slots[i].Minute < slots[j].Minute
	})

	return slots, nil
}

// FormatSchedule converts slots back to a string like "09:00,18:00".
func FormatSchedule(slots []TimeSlot) string {
	parts := make([]string, len(slots))
	for i, s := range slots {
		parts[i] = s.String()
	}
	return strings.Join(parts, ",")
}

// NextFire computes the next fire time after `now` given a set of time slots
// and a timezone location. If no slots are given, returns zero time.
func NextFire(now time.Time, slots []TimeSlot, loc *time.Location) time.Time {
	if len(slots) == 0 {
		return time.Time{}
	}

	localNow := now.In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)

	// Check today's remaining slots.
	for _, slot := range slots {
		candidate := today.Add(time.Duration(slot.Hour)*time.Hour + time.Duration(slot.Minute)*time.Minute)
		if candidate.After(now) {
			return candidate
		}
	}

	// All today's slots have passed — use first slot tomorrow.
	tomorrow := today.AddDate(0, 0, 1)
	first := slots[0]
	return tomorrow.Add(time.Duration(first.Hour)*time.Hour + time.Duration(first.Minute)*time.Minute)
}

