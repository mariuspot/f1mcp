// Package live keeps a running picture of a session from OpenF1's feed,
// whether it comes live or from a replay: positions, gaps, laps and
// sectors, tyres, pit stops, flags, weather, radio and overtakes.
package live

import (
	"encoding/json"
	"time"
)

// Record is one update from the feed: an OpenF1 record of one endpoint
// ("laps", "intervals", …), and when it became known.
type Record struct {
	Topic string
	Time  time.Time
	Data  json.RawMessage
}

// Source gives records in time order. Next returns false when there are no
// more.
type Source interface {
	Next() (Record, bool, error)
}
