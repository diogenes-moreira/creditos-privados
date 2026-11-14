package port

import (
	"context"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
)

// RUVIDTypeCedula is the registry's name for a cédula de ciudadanía, the
// document it is queried by when no other type is given.
const RUVIDTypeCedula = "CÉDULA DE CIUDADANÍA"

// RUVHolding is one predio as the RUV reports it, already mapped onto the
// domain's herd categories.
type RUVHolding struct {
	// Code is the holding's RUV code.
	Code string
	Name string

	Department   string
	Municipality string
	Village      string

	// Cycle and Year identify the vaccination cycle this record belongs to.
	// The RUV reports two cycles a year.
	Cycle int
	Year  int

	// VaccinatedAt is the reported vaccination date, as given.
	VaccinatedAt string

	Herd model.Herd
}

// RUVRancher is everything the RUV knows about one national ID: the person and
// every predio registered to them.
//
// A single ID can hold several predios and the quota is one per ID, so callers
// aggregate the herds before scoring.
type RUVRancher struct {
	NationalID string
	Name       string
	Phone      string
	Email      string

	Holdings []RUVHolding

	// HistoryComplete is whether Holdings carries every past vaccination
	// cycle, not only the current one. The registry's getRuvScoring returns
	// the current cycle alone, so its client leaves this false and the
	// history-based criteria stay unverified instead of failing.
	HistoryComplete bool
}

// LatestHoldings keeps each predio once, at its most recent vaccination cycle.
// The registry returns one record per predio and cycle, so a predio vaccinated
// in two cycles would otherwise be counted twice.
func (r RUVRancher) LatestHoldings() []RUVHolding {
	latest := map[string]int{}
	var out []RUVHolding
	for _, h := range r.Holdings {
		i, seen := latest[h.Code]
		if !seen {
			latest[h.Code] = len(out)
			out = append(out, h)
			continue
		}
		if h.Year > out[i].Year || (h.Year == out[i].Year && h.Cycle > out[i].Cycle) {
			out[i] = h
		}
	}
	return out
}

// AggregatedHerd sums every predio's animals into one herd, each predio at its
// latest cycle.
func (r RUVRancher) AggregatedHerd() model.Herd {
	var herd model.Herd
	for _, h := range r.LatestHoldings() {
		herd.Add(h.Herd)
	}
	return herd
}

// Department returns the department of the largest holding, which is the one
// whose yields best represent the operation. Ties go to the first seen.
func (r RUVRancher) Department() string {
	best, bestTotal := "", -1
	for _, h := range r.LatestHoldings() {
		if total := h.Herd.Classify().Total; total > bestTotal {
			best, bestTotal = h.Department, total
		}
	}
	return best
}

// ReportedCycles counts the distinct vaccination cycles the RUV has on record
// for this ID, which is the denominator of the compliance score.
func (r RUVRancher) ReportedCycles() int {
	seen := map[[2]int]bool{}
	for _, h := range r.Holdings {
		seen[[2]int{h.Year, h.Cycle}] = true
	}
	return len(seen)
}

// cycleIndex orders vaccination cycles: the RUV runs two a year.
func cycleIndex(year, cycle int) int { return year*2 + cycle - 1 }

// currentCycle is the most recent cycle that should already be on record at
// now. The first cycle runs around May and June and the second around
// November and December, so before May the latest one is last year's second.
func currentCycle(now time.Time) int {
	switch {
	case now.Month() >= time.November:
		return cycleIndex(now.Year(), 2)
	case now.Month() >= time.May:
		return cycleIndex(now.Year(), 1)
	default:
		return cycleIndex(now.Year()-1, 2)
	}
}

// VaccinationRecord reduces the registry's records to the figures the credit
// tiers are written against.
//
// Several definitions here are readings of the tier table, not facts the
// registry states:
//   - the latest cycle is the most recent (year, cycle) on any holding, and
//     its heads are summed over every holding reported in it;
//   - a cycle is complete when no animal in it was left unvaccinated;
//   - the holding counts as active when the rancher vaccinated in the current
//     cycle or the one before, since the registry has no status field.
func (r RUVRancher) VaccinationRecord(now time.Time) model.VaccinationRecord {
	var rec model.VaccinationRecord
	if len(r.Holdings) == 0 {
		return rec
	}

	unvaccinated := map[int]int{}
	latest := -1
	for _, h := range r.Holdings {
		idx := cycleIndex(h.Year, h.Cycle)
		unvaccinated[idx] += h.Herd.NotVaccinated
		if idx > latest {
			latest, rec.LatestYear, rec.LatestCycle = idx, h.Year, h.Cycle
		}
	}

	for _, h := range r.Holdings {
		if cycleIndex(h.Year, h.Cycle) == latest {
			rec.VaccinatedHeads += h.Herd.Classify().Total
			rec.NotVaccinated += h.Herd.NotVaccinated
		}
	}

	rec.ReportedCycles = len(unvaccinated)
	for _, n := range unvaccinated {
		if n == 0 {
			rec.CompleteCycles++
		}
	}
	rec.HoldingActive = latest >= currentCycle(now)-1
	rec.HistoryKnown = r.HistoryComplete
	return rec
}

// RUVClient reads Fedegán's vaccination registry.
//
// Implementations are infrastructure adapters. The registry is the alternative
// bureau this product underwrites on: for ranchers with no credit history it is
// the only observable record of productive capacity.
type RUVClient interface {
	// FindByNationalID returns the rancher's holdings, or nil when the registry
	// has no record for that id.
	FindByNationalID(ctx context.Context, idType, id string) (*RUVRancher, error)
}
