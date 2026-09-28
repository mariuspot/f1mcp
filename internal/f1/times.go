package f1

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// seconds parses a time such as "1:30:58.421", "1:34.183", "29.698" or
// "+5.366" into seconds. It returns nil for anything else, such as "" or
// "+1 Lap".
func seconds(s string) *float64 {
	s = strings.TrimPrefix(strings.TrimSpace(s), "+")
	if s == "" {
		return nil
	}
	total := 0.0
	for _, part := range strings.Split(s, ":") {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil
		}
		total = total*60 + v
	}
	// To the millisecond, the APIs' precision, without float noise.
	total = math.Round(total*1000) / 1000
	return &total
}

// rawSeconds reads an OpenF1 number, or nil if it's not a number.
func rawSeconds(raw json.RawMessage) *float64 {
	var v float64
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return &v
}

// rawList reads an OpenF1 list of numbers such as [Q1, Q2, Q3], with nil
// for missing entries.
func rawList(raw json.RawMessage) []*float64 {
	var vs []*float64
	if json.Unmarshal(raw, &vs) != nil {
		return nil
	}
	return vs
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// sessionTime reads Jolpica's separate date and time, which may be missing.
func sessionTime(date, clock string) time.Time {
	if t, err := time.Parse(time.RFC3339, date+"T"+clock); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.DateOnly, date); err == nil {
		return t
	}
	return time.Time{}
}
