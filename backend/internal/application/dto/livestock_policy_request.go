package dto

// LivestockUGGFactorRequest is one typology's row of the UGG conversion matrix.
// The factors are decimal strings so a rate never travels as a float.
type LivestockUGGFactorRequest struct {
	Typology  string `json:"typology" binding:"required"`
	Calves    string `json:"calves"`
	Growing   string `json:"growing"`
	Breeding  string `json:"breeding"`
	Fattening string `json:"fattening"`
	Cull      string `json:"cull"`
}

// LivestockQuotaParamsRequest carries every assumption the quota engine takes.
// Amounts, rates and factors are decimal strings: money never travels as a
// float, and a malformed value is rejected rather than silently zeroed.
type LivestockQuotaParamsRequest struct {
	PriceCalfPerKg      string `json:"priceCalfPerKg" binding:"required"`
	PriceGrowingPerKg   string `json:"priceGrowingPerKg" binding:"required"`
	PriceFatteningPerKg string `json:"priceFatteningPerKg" binding:"required"`
	WeightCalfKg        string `json:"weightCalfKg" binding:"required"`
	WeightGrowingKg     string `json:"weightGrowingKg" binding:"required"`
	WeightFatteningKg   string `json:"weightFatteningKg" binding:"required"`

	MilkingShare  string `json:"milkingShare" binding:"required"`
	PricePerLitre string `json:"pricePerLitre" binding:"required"`

	VaccineCostPerUGGYear    string `json:"vaccineCostPerUggYear"`
	SaltCostPerUGGYear       string `json:"saltCostPerUggYear"`
	SupplementCostPerUGGYear string `json:"supplementCostPerUggYear"`
	VetCostPerUGGYear        string `json:"vetCostPerUggYear"`
	DailyWage                string `json:"dailyWage"`

	LabourRegime           string `json:"labourRegime" binding:"required,oneof=auto familiar formal"`
	UGGThresholdFamily     string `json:"uggThresholdFamily" binding:"required"`
	UGGThresholdOccasional string `json:"uggThresholdOccasional" binding:"required"`
	UGGThresholdPartial    string `json:"uggThresholdPartial" binding:"required"`
	OccasionalDaysPerMonth int    `json:"occasionalDaysPerMonth" binding:"min=0,max=31"`

	FenceCostPerHaYear  string `json:"fenceCostPerHaYear"`
	InfraCostPerUGGYear string `json:"infraCostPerUggYear"`
	HectaresPerUGG      string `json:"hectaresPerUgg" binding:"required"`

	LTVPercent        string `json:"ltvPercent" binding:"required"`
	AnnualRatePct     string `json:"annualRatePct" binding:"required"`
	CapitalCostPct    string `json:"capitalCostPct" binding:"required"`
	MaxQuota          string `json:"maxQuota" binding:"required"`
	TermMonths        int    `json:"termMonths" binding:"required,min=1,max=60"`
	ApplyInputCeiling bool   `json:"applyInputCeiling"`
	InputHeadroom     string `json:"inputHeadroom"`

	Factors []LivestockUGGFactorRequest `json:"factors" binding:"required,min=1,dive"`
}

// CreateLivestockQuotaPolicyRequest registers a new parameter set.
type CreateLivestockQuotaPolicyRequest struct {
	Name        string                      `json:"name" binding:"required"`
	Description string                      `json:"description"`
	Params      LivestockQuotaParamsRequest `json:"params" binding:"required"`
	Activate    bool                        `json:"activate"`
}

// SaveLivestockQuotaPolicyVersionRequest derives a new version from an existing
// policy. This is the normal way an operator edits the engine's parameters: the
// running version keeps serving until the new one is activated.
type SaveLivestockQuotaPolicyVersionRequest struct {
	Description string                      `json:"description"`
	Params      LivestockQuotaParamsRequest `json:"params" binding:"required"`
	Activate    bool                        `json:"activate"`
}

// UpdateLivestockQuotaPolicyRequest edits a policy version in place.
type UpdateLivestockQuotaPolicyRequest struct {
	Description string                      `json:"description"`
	Params      LivestockQuotaParamsRequest `json:"params" binding:"required"`
}
