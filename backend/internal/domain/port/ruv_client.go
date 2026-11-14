package port

import (
	"context"
	"strings"
	"unicode"

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

	// Owner is the record's "propietario" as text: a string as given, a JSON
	// boolean or number as its literal, empty when null or absent. FEDEGÁN
	// does not fill it yet (always null in a sample of 1,124 records); see
	// RUVRancher.OwnsAHolding for how it is read.
	Owner string
	// Registrant and RegistrantID are the rancher's name ("ganadero") and
	// national ID ("identificacion_ganadero") on this record, which the owner
	// is compared against.
	Registrant   string
	RegistrantID string

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

// CycleHerd sums the herds of the predios recorded in one vaccination cycle,
// each predio once, and reports whether any predio was recorded in it.
// Predios whose latest record is from another cycle are left out: the tiers
// measure the herd as vaccinated in that cycle, not the last known one.
func (r RUVRancher) CycleHerd(c model.VaccinationCycle) (model.Herd, bool) {
	var herd model.Herd
	seen := map[string]bool{}
	for _, h := range r.Holdings {
		if h.Year != c.Year || h.Cycle != c.Cycle || seen[h.Code] {
			continue
		}
		seen[h.Code] = true
		herd.Add(h.Herd)
	}
	return herd, len(seen) > 0
}

// OwnsAHolding reports whether any record, of any predio and cycle, names the
// rancher as owner. Its "propietario" counts when it is an affirmative ("si",
// "sí", "true", in any case) or the rancher himself: the same name as the
// record's "ganadero", or the same digits as his national ID. Anything else,
// including empty, is not owned: over-granting is the more expensive mistake.
func (r RUVRancher) OwnsAHolding() bool {
	for _, h := range r.Holdings {
		if ownerIsRancher(h.Owner, firstNonEmpty(h.Registrant, r.Name), firstNonEmpty(h.RegistrantID, r.NationalID)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func ownerIsRancher(owner, name, nationalID string) bool {
	o := normalizeName(owner)
	switch {
	case o == "":
		return false
	case o == "SI" || o == "TRUE":
		return true
	case o == normalizeName(name):
		return true
	}
	id := digitsOf(nationalID)
	return id != "" && isIDLike(owner) && digitsOf(owner) == id
}

// accentFolder strips the accents Spanish names carry, after upper-casing.
var accentFolder = strings.NewReplacer(
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N",
	"À", "A", "È", "E", "Ì", "I", "Ò", "O", "Ù", "U", "Â", "A", "Ê", "E", "Ô", "O", "Ã", "A", "Õ", "O", "Ç", "C",
)

// normalizeName upper-cases, folds accents and collapses whitespace, so the
// same name typed two ways compares equal.
func normalizeName(s string) string {
	return accentFolder.Replace(strings.Join(strings.Fields(strings.ToUpper(s)), " "))
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, c := range s {
		if unicode.IsDigit(c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// isIDLike is true for a document number written with separators
// ("17.788.090"), so digits buried in a name or an address never match.
func isIDLike(s string) bool {
	hasDigit := false
	for _, c := range s {
		switch {
		case unicode.IsDigit(c):
			hasDigit = true
		case c == '.' || c == ',' || c == '-' || unicode.IsSpace(c):
		default:
			return false
		}
	}
	return hasDigit
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
