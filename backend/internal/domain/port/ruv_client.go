package port

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
)

// RUVHolding is one predio as the RUV reports it, already mapped onto the
// domain's herd categories.
type RUVHolding struct {
	// Code is the holding's RUV code.
	Code string
	Name string

	Department string
	Municipality string
	Village    string

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
}

// AggregatedHerd sums every holding's animals into one herd.
func (r RUVRancher) AggregatedHerd() model.Herd {
	var herd model.Herd
	for _, h := range r.Holdings {
		herd.Add(h.Herd)
	}
	return herd
}

// Department returns the department of the largest holding, which is the one
// whose yields best represent the operation. Ties go to the first seen.
func (r RUVRancher) Department() string {
	best, bestTotal := "", -1
	for _, h := range r.Holdings {
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
