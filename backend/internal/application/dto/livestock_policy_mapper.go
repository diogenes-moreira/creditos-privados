package dto

import (
	"fmt"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/shopspring/decimal"
)

// --- requests -> domain ---------------------------------------------------

// ToLivestockQuotaParams converts an edited parameter set into the domain
// parameters the engine consumes. A malformed number is an error rather than a
// silent zero: a zeroed price or threshold produces a wrong credit limit
// without failing anywhere.
func ToLivestockQuotaParams(req LivestockQuotaParamsRequest) (model.QuotaParams, error) {
	var p model.QuotaParams

	fields := []struct {
		name     string
		raw      string
		required bool
		target   *decimal.Decimal
	}{
		{"priceCalfPerKg", req.PriceCalfPerKg, true, &p.PriceCalfPerKg},
		{"priceGrowingPerKg", req.PriceGrowingPerKg, true, &p.PriceGrowingPerKg},
		{"priceFatteningPerKg", req.PriceFatteningPerKg, true, &p.PriceFatteningPerKg},
		{"weightCalfKg", req.WeightCalfKg, true, &p.WeightCalfKg},
		{"weightGrowingKg", req.WeightGrowingKg, true, &p.WeightGrowingKg},
		{"weightFatteningKg", req.WeightFatteningKg, true, &p.WeightFatteningKg},
		{"milkingShare", req.MilkingShare, true, &p.MilkingShare},
		{"pricePerLitre", req.PricePerLitre, true, &p.PricePerLitre},
		{"vaccineCostPerUggYear", req.VaccineCostPerUGGYear, false, &p.VaccineCostPerUGGYear},
		{"saltCostPerUggYear", req.SaltCostPerUGGYear, false, &p.SaltCostPerUGGYear},
		{"supplementCostPerUggYear", req.SupplementCostPerUGGYear, false, &p.SupplementCostPerUGGYear},
		{"vetCostPerUggYear", req.VetCostPerUGGYear, false, &p.VetCostPerUGGYear},
		{"dailyWage", req.DailyWage, false, &p.DailyWage},
		{"uggThresholdFamily", req.UGGThresholdFamily, true, &p.UGGThresholdFamily},
		{"uggThresholdOccasional", req.UGGThresholdOccasional, true, &p.UGGThresholdOccasional},
		{"uggThresholdPartial", req.UGGThresholdPartial, true, &p.UGGThresholdPartial},
		{"fenceCostPerHaYear", req.FenceCostPerHaYear, false, &p.FenceCostPerHaYear},
		{"infraCostPerUggYear", req.InfraCostPerUGGYear, false, &p.InfraCostPerUGGYear},
		{"hectaresPerUgg", req.HectaresPerUGG, true, &p.HectaresPerUGG},
		{"ltvPercent", req.LTVPercent, true, &p.LTVPercent},
		{"annualRatePct", req.AnnualRatePct, true, &p.AnnualRatePct},
		{"capitalCostPct", req.CapitalCostPct, true, &p.CapitalCostPct},
		{"maxQuota", req.MaxQuota, true, &p.MaxQuota},
		{"inputHeadroom", req.InputHeadroom, false, &p.InputHeadroom},
	}
	for _, f := range fields {
		var (
			v   decimal.Decimal
			err error
		)
		if f.required {
			v, err = requiredDecimal(f.name, f.raw)
		} else {
			v, err = optionalDecimal(f.name, f.raw)
		}
		if err != nil {
			return p, err
		}
		*f.target = v
	}

	p.LabourRegime = model.LabourRegime(req.LabourRegime)
	p.OccasionalDaysPerMonth = req.OccasionalDaysPerMonth
	p.TermMonths = req.TermMonths
	p.ApplyInputCeiling = req.ApplyInputCeiling

	factors, err := toUGGFactorMatrix(req.Factors)
	if err != nil {
		return p, err
	}
	p.UGGFactors = factors
	return p, nil
}

func toUGGFactorMatrix(reqs []LivestockUGGFactorRequest) (map[model.LivestockTypology]model.UGGFactors, error) {
	matrix := make(map[model.LivestockTypology]model.UGGFactors, len(reqs))
	for _, r := range reqs {
		typology := model.LivestockTypology(r.Typology)
		if _, duplicate := matrix[typology]; duplicate {
			return nil, fmt.Errorf("duplicate typology in the UGG matrix: %s", r.Typology)
		}
		var row model.UGGFactors
		cells := []struct {
			name   string
			raw    string
			target *decimal.Decimal
		}{
			{"calves", r.Calves, &row.Calves},
			{"growing", r.Growing, &row.Growing},
			{"breeding", r.Breeding, &row.Breeding},
			{"fattening", r.Fattening, &row.Fattening},
			{"cull", r.Cull, &row.Cull},
		}
		for _, cell := range cells {
			v, err := optionalDecimal(fmt.Sprintf("%s %s factor", r.Typology, cell.name), cell.raw)
			if err != nil {
				return nil, err
			}
			*cell.target = v
		}
		matrix[typology] = row
	}
	return matrix, nil
}

// --- domain -> responses --------------------------------------------------

func ToLivestockUGGFactorResponses(p *model.LivestockQuotaPolicy) []LivestockUGGFactorResponse {
	out := make([]LivestockUGGFactorResponse, 0, len(p.Factors))
	for i := range p.Factors {
		f := &p.Factors[i]
		out = append(out, LivestockUGGFactorResponse{
			Typology:  string(f.Typology),
			Calves:    f.Calves.StringFixed(4),
			Growing:   f.Growing.StringFixed(4),
			Breeding:  f.Breeding.StringFixed(4),
			Fattening: f.Fattening.StringFixed(4),
			Cull:      f.Cull.StringFixed(4),
		})
	}
	return out
}

func ToLivestockQuotaPolicyResponse(p *model.LivestockQuotaPolicy) LivestockQuotaPolicyResponse {
	typologies := make([]string, 0, len(model.OrderedTypologies))
	for _, t := range model.OrderedTypologies {
		typologies = append(typologies, string(t))
	}
	return LivestockQuotaPolicyResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		IsActive:    p.IsActive,

		PriceCalfPerKg:      p.PriceCalfPerKg.StringFixed(2),
		PriceGrowingPerKg:   p.PriceGrowingPerKg.StringFixed(2),
		PriceFatteningPerKg: p.PriceFatteningPerKg.StringFixed(2),
		WeightCalfKg:        p.WeightCalfKg.StringFixed(2),
		WeightGrowingKg:     p.WeightGrowingKg.StringFixed(2),
		WeightFatteningKg:   p.WeightFatteningKg.StringFixed(2),

		MilkingShare:  p.MilkingShare.StringFixed(4),
		PricePerLitre: p.PricePerLitre.StringFixed(2),

		VaccineCostPerUGGYear:    p.VaccineCostPerUGGYear.StringFixed(2),
		SaltCostPerUGGYear:       p.SaltCostPerUGGYear.StringFixed(2),
		SupplementCostPerUGGYear: p.SupplementCostPerUGGYear.StringFixed(2),
		VetCostPerUGGYear:        p.VetCostPerUGGYear.StringFixed(2),
		DailyWage:                p.DailyWage.StringFixed(2),

		LabourRegime:           string(p.LabourRegime),
		UGGThresholdFamily:     p.UGGThresholdFamily.StringFixed(2),
		UGGThresholdOccasional: p.UGGThresholdOccasional.StringFixed(2),
		UGGThresholdPartial:    p.UGGThresholdPartial.StringFixed(2),
		OccasionalDaysPerMonth: p.OccasionalDaysPerMonth,

		FenceCostPerHaYear:  p.FenceCostPerHaYear.StringFixed(2),
		InfraCostPerUGGYear: p.InfraCostPerUGGYear.StringFixed(2),
		HectaresPerUGG:      p.HectaresPerUGG.StringFixed(4),

		LTVPercent:        p.LTVPercent.StringFixed(4),
		AnnualRatePct:     p.AnnualRatePct.StringFixed(4),
		CapitalCostPct:    p.CapitalCostPct.StringFixed(4),
		MaxQuota:          p.MaxQuota.StringFixed(2),
		TermMonths:        p.TermMonths,
		ApplyInputCeiling: p.ApplyInputCeiling,
		InputHeadroom:     p.InputHeadroom.StringFixed(4),

		Factors:    ToLivestockUGGFactorResponses(p),
		Typologies: typologies,
		CreatedAt:  p.CreatedAt.Format(timeLayout),
		UpdatedAt:  p.UpdatedAt.Format(timeLayout),
	}
}

func ToLivestockQuotaPolicyResponses(policies []model.LivestockQuotaPolicy) []LivestockQuotaPolicyResponse {
	out := make([]LivestockQuotaPolicyResponse, len(policies))
	for i := range policies {
		out[i] = ToLivestockQuotaPolicyResponse(&policies[i])
	}
	return out
}
