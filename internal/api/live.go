package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/mariuspot/f1mcp/internal/live"
	"github.com/mariuspot/f1mcp/internal/replay"
)

// LiveResponse is the live page's view: what's playing, the state of the
// session, and events since the ones the page has.
type LiveResponse struct {
	Status   replay.Status `json:"status"`
	Snapshot live.Snapshot `json:"snapshot"`
	Events   []live.Event  `json:"events"`
}

type replayRequest struct {
	Race    string  `json:"race"`
	Speed   float64 `json:"speed"`
	FromLap int     `json:"from_lap"`
	Stop    bool    `json:"stop"`
}

// LiveHandler serves the live page's API from a replay player:
//
//	GET  /api/live?since=<event id>  status, snapshot and new events
//	GET  /api/live/races             the sessions that can be replayed
//	POST /api/live/replay            {race, speed, from_lap} or {stop: true}
func LiveHandler(p *replay.Player) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/live", func(w http.ResponseWriter, r *http.Request) {
		since, _ := strconv.Atoi(r.URL.Query().Get("since"))
		st := p.State()
		events := st.EventsSince(since)
		if events == nil {
			events = []live.Event{}
		}
		writeJSON(w, http.StatusOK, LiveResponse{Status: p.Status(), Snapshot: st.Snapshot(), Events: events})
	})
	mux.HandleFunc("GET /api/live/races", func(w http.ResponseWriter, r *http.Request) {
		races, err := p.Races()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody{err.Error()})
			return
		}
		if races == nil {
			races = []replay.Race{}
		}
		writeJSON(w, http.StatusOK, races)
	})
	mux.HandleFunc("POST /api/live/replay", func(w http.ResponseWriter, r *http.Request) {
		var req replayRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{err.Error()})
			return
		}
		if req.Stop {
			p.Stop()
		} else if err := p.Start(req.Race, req.Speed, req.FromLap); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorBody{err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, p.Status())
	})
	return mux
}
