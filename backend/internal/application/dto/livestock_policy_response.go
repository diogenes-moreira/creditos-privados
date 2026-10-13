package dto

// LivestockUGGFactorResponse is one typology's row of the UGG matrix.
type LivestockUGGFactorResponse struct {
	Typology  string `json:"typology"`
	Calves    string `json:"calves"`
	Growing   string `json:"growing"`
	Breeding  string `json:"breeding"`
	Fattening string `json:"fattening"`
	Cull      string `json:"cull"`
}

// LivestockQuotaPolicyResponse is one versioned parameter set of the livestock
// quota engine, as the admin console edits it.
type LivestockQuotaPolicyResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Description string `json:"description,omitempty"`
	IsActive    bool   `json:"isActive"`

	PriceCalfPerKg      string `json:"priceCalfPerKg"`
	PriceGrowingPerKg   string `json:"priceGrowingPerKg"`
	PriceFatteningPerKg string `json:"priceFatteningPerKg"`
	WeightCalfKg        string `json:"weightCalfKg"`
	WeightGrowingKg     string `json:"weightGrowingKg"`
	WeightFatteningKg   string `json:"weightFatteningKg"`

	MilkingShare  string `json:"milkingShare"`
	PricePerLitre string `json:"pricePerLitre"`

	VaccineCostPerUGGYear    string `json:"vaccineCostPerUggYear"`
	SaltCostPerUGGYear       string `json:"saltCostPerUggYear"`
	SupplementCostPerUGGYear string `json:"supplementCostPerUggYear"`
	VetCostPerUGGYear        string `json:"vetCostPerUggYear"`
	DailyWage                string `json:"dailyWage"`

	LabourRegime           string `json:"labourRegime"`
	UGGThresholdFamily     string `json:"uggThresholdFamily"`
	UGGThresholdOccasional string `json:"uggThresholdOccasional"`
	UGGThresholdPartial    string `json:"uggThresholdPartial"`
	OccasionalDaysPerMonth int    `json:"occasionalDaysPerMonth"`

	FenceCostPerHaYear  string `json:"fenceCostPerHaYear"`
	InfraCostPerUGGYear string `json:"infraCostPerUggYear"`
	HectaresPerUGG      string `json:"hectaresPerUgg"`

	LTVPercent        string `json:"ltvPercent"`
	AnnualRatePct     string `json:"annualRatePct"`
	CapitalCostPct    string `json:"capitalCostPct"`
	MaxQuota          string `json:"maxQuota"`
	TermMonths        int    `json:"termMonths"`
	ApplyInputCeiling bool   `json:"applyInputCeiling"`
	InputHeadroom     string `json:"inputHeadroom"`

	Factors    []LivestockUGGFactorResponse `json:"factors"`
	Typologies []string                     `json:"typologies"`
	CreatedAt  string                       `json:"createdAt"`
	UpdatedAt  string                       `json:"updatedAt"`
}
