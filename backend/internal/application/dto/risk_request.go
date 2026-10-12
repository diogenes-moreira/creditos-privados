package dto

// SubmitRiskProfileRequest carries a client's credit-risk data. Monetary fields
// are strings so decimal precision is not lost to JSON float parsing.
type SubmitRiskProfileRequest struct {
	MonthlyIncome        string `json:"monthlyIncome" binding:"required"`
	AdditionalIncome     string `json:"additionalIncome"`
	IncomeVerified       bool   `json:"incomeVerified"`
	EmploymentType       string `json:"employmentType" binding:"required,oneof=employee self_employed business_owner retired informal unemployed"`
	EmployerName         string `json:"employerName"`
	MonthsEmployed       int    `json:"monthsEmployed" binding:"min=0"`
	MonthlyExpenses      string `json:"monthlyExpenses"`
	ExistingDebtPayments string `json:"existingDebtPayments"`
	Dependents           int    `json:"dependents" binding:"min=0"`
	HousingStatus        string `json:"housingStatus" binding:"required,oneof=owned mortgaged rented family other"`
	HasBankAccount       bool   `json:"hasBankAccount"`
	BureauScore          *int   `json:"bureauScore" binding:"omitempty,min=0,max=1000"`
	AdverseRecords       int    `json:"adverseRecords" binding:"min=0"`
	Notes                string `json:"notes"`
}

// AdminSubmitRiskProfileRequest is the operator-side variant, which names the
// client instead of taking it from the authenticated user.
type AdminSubmitRiskProfileRequest struct {
	ClientID string `json:"clientId" binding:"required,uuid"`
	SubmitRiskProfileRequest
}

// SimulateRiskRequest scores hypothetical data for pre-qualification. It needs
// an age because eligibility depends on it and there is no client record yet.
type SimulateRiskRequest struct {
	Age int `json:"age" binding:"required,min=16,max=120"`
	SubmitRiskProfileRequest
}

// RiskGradeBandRequest is one score band of an analysis model.
type RiskGradeBandRequest struct {
	Grade                string `json:"grade" binding:"required,oneof=A B C D E"`
	MinScore             int    `json:"minScore" binding:"min=0,max=100"`
	InterestSpread       string `json:"interestSpread"`
	ExposureFactor       string `json:"exposureFactor"`
	MaxInstallments      int    `json:"maxInstallments" binding:"min=0,max=60"`
	Decision             string `json:"decision" binding:"required,oneof=approve manual_review decline"`
	ProbabilityOfDefault string `json:"probabilityOfDefault"`
}

// RiskModelParamsRequest carries the numeric guardrails of an analysis model.
type RiskModelParamsRequest struct {
	MinAge                  int    `json:"minAge" binding:"required,min=18,max=120"`
	MaxAge                  int    `json:"maxAge" binding:"required,min=19,max=120"`
	MinMonthlyIncome        string `json:"minMonthlyIncome"`
	MaxAdverseRecords       int    `json:"maxAdverseRecords" binding:"min=0"`
	MaxDTI                  string `json:"maxDti" binding:"required"`
	MaxPaymentToIncome      string `json:"maxPaymentToIncome" binding:"required"`
	UnverifiedIncomeHaircut string `json:"unverifiedIncomeHaircut"`
	ExpensePerDependent     string `json:"expensePerDependent"`
	AbsoluteMaxLimit        string `json:"absoluteMaxLimit" binding:"required"`
	MaxInstallments         int    `json:"maxInstallments" binding:"required,min=1,max=60"`
	LimitRoundingStep       string `json:"limitRoundingStep"`
	BaseInterestRate        string `json:"baseInterestRate" binding:"required"`
}

// CreateRiskModelRequest registers a new analysis model.
type CreateRiskModelRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Prompt      string                 `json:"prompt" binding:"required"`
	Description string                 `json:"description"`
	Params      RiskModelParamsRequest `json:"params" binding:"required"`
	Bands       []RiskGradeBandRequest `json:"bands" binding:"required,min=1,dive"`
	Activate    bool                   `json:"activate"`
}

// SaveRiskModelVersionRequest derives a new version from an existing model.
// This is the normal way an operator edits the analysis prompt: the running
// version keeps serving until the new one is activated.
type SaveRiskModelVersionRequest struct {
	Prompt      string                 `json:"prompt" binding:"required"`
	Description string                 `json:"description"`
	Params      RiskModelParamsRequest `json:"params" binding:"required"`
	Bands       []RiskGradeBandRequest `json:"bands" binding:"omitempty,dive"`
	Activate    bool                   `json:"activate"`
}

// UpdateRiskModelRequest edits a model version in place.
type UpdateRiskModelRequest struct {
	Prompt      string                 `json:"prompt"`
	Description string                 `json:"description"`
	Params      RiskModelParamsRequest `json:"params" binding:"required"`
	Bands       []RiskGradeBandRequest `json:"bands" binding:"omitempty,dive"`
}

// PreviewPromptRequest renders a candidate prompt without saving it.
type PreviewPromptRequest struct {
	Prompt   string `json:"prompt" binding:"required"`
	ClientID string `json:"clientId" binding:"omitempty,uuid"`
}
