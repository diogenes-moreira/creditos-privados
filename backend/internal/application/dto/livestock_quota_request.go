package dto

// LivestockHerdRequest is one holding's (predio) animal count from the RUV.
type LivestockHerdRequest struct {
	FemalesUnder3Months int `json:"femalesUnder3Months" binding:"min=0"`
	Females3To8Months   int `json:"females3To8Months" binding:"min=0"`
	Females8To12Months  int `json:"females8To12Months" binding:"min=0"`
	Females1To2Years    int `json:"females1To2Years" binding:"min=0"`
	Females2To3Years    int `json:"females2To3Years" binding:"min=0"`
	Females3To5Years    int `json:"females3To5Years" binding:"min=0"`
	FemalesOver5Years   int `json:"femalesOver5Years" binding:"min=0"`
	MalesUnder3Months   int `json:"malesUnder3Months" binding:"min=0"`
	Males3To8Months     int `json:"males3To8Months" binding:"min=0"`
	Males8To12Months    int `json:"males8To12Months" binding:"min=0"`
	Males1To2Years      int `json:"males1To2Years" binding:"min=0"`
	Males2To3Years      int `json:"males2To3Years" binding:"min=0"`
	MalesOver3Years     int `json:"malesOver3Years" binding:"min=0"`
	// NotVaccinated is the registry's unvaccinated total for the holding.
	NotVaccinated int `json:"notVaccinated" binding:"min=0"`
}

// LivestockQuotaRequest is a rancher's RUV data. Holdings are summed: the quota
// is one per national ID.
type LivestockQuotaRequest struct {
	NationalID string `json:"nationalId" binding:"required,max=30"`
	Department string `json:"department" binding:"max=60"`
	// Reference is the caller's identifier for the request (e.g. its onboarding ID).
	Reference string                 `json:"reference" binding:"max=100"`
	Holdings  []LivestockHerdRequest `json:"holdings" binding:"required,min=1,dive"`
	// Tenure is "Propia" or "Arrendada", as the RUV reports it.
	Tenure string `json:"tenure" binding:"required,oneof=Propia Arrendada"`
	// HoldingStatus is the RUV's holding status; "N" (active) scores best.
	HoldingStatus   string `json:"holdingStatus" binding:"required,max=10"`
	SeniorityCycles int    `json:"seniorityCycles" binding:"min=0"`
	CompletedCycles int    `json:"completedCycles" binding:"min=0"`
	ReportedCycles  int    `json:"reportedCycles" binding:"min=0"`
}
