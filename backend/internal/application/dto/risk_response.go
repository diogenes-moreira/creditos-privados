package dto

// RiskProfileResponse is a client's credit-risk data as returned by the API.
type RiskProfileResponse struct {
	ID                   string `json:"id"`
	ClientID             string `json:"clientId"`
	ClientName           string `json:"clientName,omitempty"`
	MonthlyIncome        string `json:"monthlyIncome"`
	AdditionalIncome     string `json:"additionalIncome"`
	GrossMonthlyIncome   string `json:"grossMonthlyIncome"`
	IncomeVerified       bool   `json:"incomeVerified"`
	EmploymentType       string `json:"employmentType"`
	EmployerName         string `json:"employerName,omitempty"`
	MonthsEmployed       int    `json:"monthsEmployed"`
	MonthlyExpenses      string `json:"monthlyExpenses"`
	ExistingDebtPayments string `json:"existingDebtPayments"`
	Dependents           int    `json:"dependents"`
	HousingStatus        string `json:"housingStatus"`
	HasBankAccount       bool   `json:"hasBankAccount"`
	BureauScore          *int   `json:"bureauScore,omitempty"`
	BureauScoreScale     int    `json:"bureauScoreScale"`
	AdverseRecords       int    `json:"adverseRecords"`
	DebtToIncomeRatio    string `json:"debtToIncomeRatio"`
	ExpenseRatio         string `json:"expenseRatio"`
	Notes                string `json:"notes,omitempty"`
	Source               string `json:"source"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
}

// RiskFactorResponse is one explanatory driver of an assessment.
type RiskFactorResponse struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Value   string `json:"value,omitempty"`
	Impact  string `json:"impact"`
	Weight  string `json:"weight,omitempty"`
	Points  int    `json:"points"`
	Comment string `json:"comment,omitempty"`
}

// RiskAssessmentResponse is a stored analysis outcome.
type RiskAssessmentResponse struct {
	ID                      string               `json:"id"`
	ClientID                string               `json:"clientId"`
	ClientName              string               `json:"clientName,omitempty"`
	RiskProfileID           string               `json:"riskProfileId"`
	RiskModelID             string               `json:"riskModelId"`
	RiskModelName           string               `json:"riskModelName"`
	ModelVersion            int                  `json:"modelVersion"`
	Analyzer                string               `json:"analyzer"`
	Score                   int                  `json:"score"`
	Grade                   string               `json:"grade"`
	ProbabilityOfDefault    string               `json:"probabilityOfDefault"`
	Decision                string               `json:"decision"`
	MaxMonthlyPayment       string               `json:"maxMonthlyPayment"`
	RecommendedLimit        string               `json:"recommendedLimit"`
	RecommendedInstallments int                  `json:"recommendedInstallments"`
	RecommendedInterestRate string               `json:"recommendedInterestRate"`
	Summary                 string               `json:"summary,omitempty"`
	PolicyNotes             []string             `json:"policyNotes,omitempty"`
	Factors                 []RiskFactorResponse `json:"factors,omitempty"`
	CreatedAt               string               `json:"createdAt"`
}

// RiskEvaluationResponse is an unsaved analysis outcome, used by simulations.
type RiskEvaluationResponse struct {
	Score                   int                  `json:"score"`
	Grade                   string               `json:"grade"`
	ProbabilityOfDefault    string               `json:"probabilityOfDefault"`
	Decision                string               `json:"decision"`
	MaxMonthlyPayment       string               `json:"maxMonthlyPayment"`
	RecommendedLimit        string               `json:"recommendedLimit"`
	RecommendedInstallments int                  `json:"recommendedInstallments"`
	RecommendedInterestRate string               `json:"recommendedInterestRate"`
	Summary                 string               `json:"summary,omitempty"`
	PolicyNotes             []string             `json:"policyNotes,omitempty"`
	Factors                 []RiskFactorResponse `json:"factors,omitempty"`
	Analyzer                string               `json:"analyzer"`
}

// CreditAvailabilityResponse is the credit headroom the risk module derives for
// a client: what risk recommends, what has been granted, and what is left.
type CreditAvailabilityResponse struct {
	ClientID                string                  `json:"clientId"`
	Decision                string                  `json:"decision"`
	RecommendedLimit        string                  `json:"recommendedLimit"`
	GrantedLimit            string                  `json:"grantedLimit"`
	UsedAmount              string                  `json:"usedAmount"`
	AvailableAmount         string                  `json:"availableAmount"`
	RecommendedInstallments int                     `json:"recommendedInstallments"`
	RecommendedInterestRate string                  `json:"recommendedInterestRate"`
	Stale                   bool                    `json:"stale"`
	Assessment              *RiskAssessmentResponse `json:"assessment,omitempty"`
}

// RiskGradeBandResponse is one score band of an analysis model.
type RiskGradeBandResponse struct {
	Grade                string `json:"grade"`
	MinScore             int    `json:"minScore"`
	InterestSpread       string `json:"interestSpread"`
	ExposureFactor       string `json:"exposureFactor"`
	MaxInstallments      int    `json:"maxInstallments"`
	Decision             string `json:"decision"`
	ProbabilityOfDefault string `json:"probabilityOfDefault"`
}

// RiskModelResponse is an analysis model: the operator-editable prompt plus the
// guardrails that bound whatever the analysis recommends.
type RiskModelResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Prompt      string `json:"prompt"`
	Description string `json:"description,omitempty"`
	IsActive    bool   `json:"isActive"`

	MinAge                  int    `json:"minAge"`
	MaxAge                  int    `json:"maxAge"`
	MinMonthlyIncome        string `json:"minMonthlyIncome"`
	MaxAdverseRecords       int    `json:"maxAdverseRecords"`
	MaxDTI                  string `json:"maxDti"`
	MaxPaymentToIncome      string `json:"maxPaymentToIncome"`
	UnverifiedIncomeHaircut string `json:"unverifiedIncomeHaircut"`
	ExpensePerDependent     string `json:"expensePerDependent"`
	AbsoluteMaxLimit        string `json:"absoluteMaxLimit"`
	MaxInstallments         int    `json:"maxInstallments"`
	LimitRoundingStep       string `json:"limitRoundingStep"`
	BaseInterestRate        string `json:"baseInterestRate"`

	Bands        []RiskGradeBandResponse `json:"bands"`
	Placeholders []string                `json:"placeholders"`
	CreatedAt    string                  `json:"createdAt"`
	UpdatedAt    string                  `json:"updatedAt"`
}

// PromptPreviewResponse shows an operator exactly what an analysis would
// receive for a candidate prompt, without saving or sending anything.
type PromptPreviewResponse struct {
	Prompt       string                  `json:"prompt"`
	Rendered     string                  `json:"rendered"`
	Applicant    string                  `json:"applicant"`
	Analyzer     string                  `json:"analyzer"`
	Placeholders []string                `json:"placeholders"`
	Warnings     []string                `json:"warnings,omitempty"`
	Baseline     *RiskEvaluationResponse `json:"baseline,omitempty"`
}

// RiskAnalyzerInfoResponse tells the console which analysis engine is in use.
type RiskAnalyzerInfoResponse struct {
	Analyzer     string   `json:"analyzer"`
	PromptDriven bool     `json:"promptDriven"`
	Placeholders []string `json:"placeholders"`
}
