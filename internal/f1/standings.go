package f1

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/mariuspot/f1mcp/internal/jolpica"
)

// Championships.
const (
	DriversChampionship = "drivers"
	TeamsChampionship   = "teams"
)

// Standings returns the drivers' or teams' championship after a round, or
// the latest if round is 0.
func (s *Service) Standings(ctx context.Context, year, round int, championship string) ([]Standing, int, error) {
	year = s.year(year)
	r := ""
	if round > 0 {
		r = strconv.Itoa(round)
	}
	type answer struct {
		standings []Standing
		round     int
	}
	key := fmt.Sprintf("standings/%d/%s/%s", year, r, championship)
	a, err := memo(s, key, year, func() (answer, error) {
		var (
			list *jolpica.StandingsList
			err  error
		)
		switch championship {
		case DriversChampionship, "":
			list, err = s.jol.DriverStandings(ctx, strconv.Itoa(year), r)
		case TeamsChampionship:
			list, err = s.jol.ConstructorStandings(ctx, strconv.Itoa(year), r)
		default:
			return answer{}, fmt.Errorf("unknown championship %q (want drivers or teams)", championship)
		}
		if errors.Is(err, jolpica.ErrNotFound) {
			return answer{}, fmt.Errorf("no %d standings yet", year)
		}
		if err != nil {
			return answer{}, err
		}
		var out []Standing
		for _, d := range list.DriverStandings {
			ref := driverRef(d.Driver, 0)
			team := ""
			if len(d.Constructors) > 0 {
				team = d.Constructors[len(d.Constructors)-1].Name
			}
			out = append(out, Standing{Position: atoi(d.Position), Driver: &ref, Team: team, Points: atof(d.Points), Wins: atoi(d.Wins)})
		}
		for _, c := range list.ConstructorStandings {
			out = append(out, Standing{Position: atoi(c.Position), Team: c.Constructor.Name, Points: atof(c.Points), Wins: atoi(c.Wins)})
		}
		return answer{out, atoi(list.Round)}, nil
	})
	return a.standings, a.round, err
}
