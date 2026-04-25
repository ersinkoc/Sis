package policy

import (
	"fmt"
	"strings"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

type ClientResolver interface {
	GroupOf(clientKey string) string
}

type StaticClientResolver map[string]string

func (r StaticClientResolver) GroupOf(clientKey string) string {
	if group := r[clientKey]; group != "" {
		return group
	}
	return "default"
}

type Identity struct {
	Key  string
	Type string
	IP   string
}

type Group struct {
	Name          string
	BaseLists     []string
	AllowlistTree *Domains
	Schedules     []CompiledSchedule
}

type CompiledSchedule struct {
	Name  string
	Days  daySet
	From  minutesOfDay
	To    minutesOfDay
	Block []string
}

type daySet uint8
type minutesOfDay int

const (
	daySunday daySet = 1 << iota
	dayMonday
	dayTuesday
	dayWednesday
	dayThursday
	dayFriday
	daySaturday
)

func CompileGroups(groups []config.Group) (map[string]*Group, error) {
	out := make(map[string]*Group, len(groups))
	for _, raw := range groups {
		if raw.Name == "" {
			return nil, fmt.Errorf("group.name: required")
		}
		g := &Group{
			Name:          raw.Name,
			BaseLists:     append([]string(nil), raw.Blocklists...),
			AllowlistTree: NewDomains(),
		}
		for _, domain := range raw.Allowlist {
			if !g.AllowlistTree.Add(domain) {
				return nil, fmt.Errorf("groups[%s].allowlist: invalid domain %q", raw.Name, domain)
			}
		}
		for _, schedule := range raw.Schedules {
			compiled, err := CompileSchedule(schedule)
			if err != nil {
				return nil, fmt.Errorf("groups[%s].schedules[%s]: %w", raw.Name, schedule.Name, err)
			}
			g.Schedules = append(g.Schedules, compiled)
		}
		out[g.Name] = g
	}
	if out["default"] == nil {
		return nil, fmt.Errorf("groups: default group is required")
	}
	return out, nil
}

func CompileSchedule(raw config.Schedule) (CompiledSchedule, error) {
	days, err := parseDays(raw.Days)
	if err != nil {
		return CompiledSchedule{}, err
	}
	from, err := parseClock(raw.From)
	if err != nil {
		return CompiledSchedule{}, fmt.Errorf("from: %w", err)
	}
	to, err := parseClock(raw.To)
	if err != nil {
		return CompiledSchedule{}, fmt.Errorf("to: %w", err)
	}
	return CompiledSchedule{
		Name: raw.Name, Days: days, From: from, To: to,
		Block: append([]string(nil), raw.Block...),
	}, nil
}

func (s CompiledSchedule) ActiveAt(now time.Time, tz *time.Location) bool {
	if tz == nil {
		tz = time.Local
	}
	local := now.In(tz)
	minute := minutesOfDay(local.Hour()*60 + local.Minute())
	today := dayFor(local.Weekday())
	if s.From == s.To {
		return s.Days.has(today)
	}
	if s.From < s.To {
		return s.Days.has(today) && minute >= s.From && minute < s.To
	}
	if minute >= s.From {
		return s.Days.has(today)
	}
	yesterday := dayFor(local.AddDate(0, 0, -1).Weekday())
	return s.Days.has(yesterday) && minute < s.To
}

func parseDays(raw []string) (daySet, error) {
	var out daySet
	for _, day := range raw {
		switch strings.ToLower(day) {
		case "all":
			out |= daySunday | dayMonday | dayTuesday | dayWednesday | dayThursday | dayFriday | daySaturday
		case "weekday":
			out |= dayMonday | dayTuesday | dayWednesday | dayThursday | dayFriday
		case "weekend":
			out |= daySaturday | daySunday
		case "sun":
			out |= daySunday
		case "mon":
			out |= dayMonday
		case "tue":
			out |= dayTuesday
		case "wed":
			out |= dayWednesday
		case "thu":
			out |= dayThursday
		case "fri":
			out |= dayFriday
		case "sat":
			out |= daySaturday
		default:
			return 0, fmt.Errorf("unknown day token %q", day)
		}
	}
	if out == 0 {
		return 0, fmt.Errorf("days: required")
	}
	return out, nil
}

func parseClock(raw string) (minutesOfDay, error) {
	parsed, err := time.Parse("15:04", raw)
	if err != nil {
		return 0, err
	}
	return minutesOfDay(parsed.Hour()*60 + parsed.Minute()), nil
}

func dayFor(w time.Weekday) daySet {
	switch w {
	case time.Sunday:
		return daySunday
	case time.Monday:
		return dayMonday
	case time.Tuesday:
		return dayTuesday
	case time.Wednesday:
		return dayWednesday
	case time.Thursday:
		return dayThursday
	case time.Friday:
		return dayFriday
	default:
		return daySaturday
	}
}

func (d daySet) has(day daySet) bool {
	return d&day != 0
}
