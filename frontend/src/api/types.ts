// ---- Auth ----
export interface RegisterRequest {
  email: string;
  firstName: string;
  lastName: string;
  dni: string;
  cuit: string;
  dateOfBirth: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
  isPEP: boolean;
}

export interface GoogleLoginRequest {
  idToken: string;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export interface User {
  id: string;
  email: string;
  role: "admin" | "client" | "vendor" | "advisor";
  firstName?: string;
  lastName?: string;
}

// ---- Profile ----
export interface Profile {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  dni: string;
  cuit: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
  mercadoPagoLink?: string;
  createdAt: string;
}

export interface UpdateProfileRequest {
  firstName: string;
  lastName: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
}

export interface AdminUpdateClientRequest {
  firstName: string;
  lastName: string;
  dni: string;
  cuit: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
  isPEP: boolean;
}

export interface UpdateMercadoPagoRequest {
  alias: string;
  cvu: string;
}

// ---- Account ----
export interface Account {
  id: string;
  clientId: string;
  balance: string;
}

export interface Movement {
  id: string;
  type: string;
  amount: string;
  balanceAfter: string;
  description: string;
  reference?: string;
  createdAt: string;
}

// ---- Credit Lines ----
export interface CreditLine {
  id: string;
  clientId: string;
  clientName?: string;
  maxAmount: string;
  usedAmount: string;
  availableAmount: string;
  interestRate: string;
  maxInstallments: number;
  recalculateOnPrepay: boolean;
  status: string;
  approvedAt?: string;
  rejectionReason?: string;
  createdAt: string;
}

export interface CreateCreditLineRequest {
  clientId: string;
  maxAmount: string;
  interestRate: string;
  maxInstallments: number;
  recalculateOnPrepay?: boolean;
}

export interface UpdateCreditLineRequest {
  maxAmount: string;
  recalculateOnPrepay?: boolean;
}

// ---- Loans ----
export interface CancellationSettlement {
  pendingCapital: string;
  accumulatedInterest: string;
  accumulatedIVA: string;
  total: string;
}

export interface Loan {
  id: string;
  clientId: string;
  clientName?: string;
  creditLineId: string;
  principal: string;
  interestRate: string;
  numInstallments: number;
  amortizationType: string;
  status: string;
  totalPaid: string;
  totalRemaining: string;
  disbursedAt?: string;
  cancellationSettlement?: CancellationSettlement;
  installments?: Installment[];
  createdAt: string;
}

export interface LoanDetail extends Loan {
  installments: Installment[];
}

export interface Installment {
  id: string;
  number: number;
  dueDate: string;
  capitalAmount: string;
  interestAmount: string;
  ivaAmount: string;
  totalAmount: string;
  paidAmount: string;
  remainingAmount: string;
  penaltyApplied: boolean;
  status: string;
  paidAt?: string;
}

export interface LoanRequest {
  creditLineId: string;
  amount: number;
  installments: number;
  amortizationType: "french" | "german";
}

export interface SimulateRequest {
  amount: number;
  installments: number;
  interestRate: number;
  amortizationType: "french" | "german";
}

export interface SimulationResult {
  principal: string;
  interestRate: string;
  totalInterest: string;
  totalPayment: string;
  installments: Installment[];
}

// ---- Payments ----
export interface Payment {
  id: string;
  loanId: string;
  installmentId?: string;
  amount: string;
  method: string;
  reference?: string;
  isAdjustment: boolean;
  adjustmentNote?: string;
  createdAt: string;
}

export interface RecordPaymentRequest {
  amount: string;
  method: string;
  installmentId?: string;
}

export interface AdjustPaymentRequest {
  amount: number;
  reason: string;
}

// ---- Admin: Clients ----
export interface Client {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  dni: string;
  cuit: string;
  dateOfBirth: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
  comments?: string;
  isPEP: boolean;
  ivaRate?: string;
  isBlocked: boolean;
  createdAt: string;
}

// ---- Admin: Dashboard ----
export interface PortfolioSummary {
  totalClients: number;
  activeLoans: number;
  totalDisbursed: number;
  totalCollected: number;
  pendingApprovals: number;
}

export interface DelinquencySummary {
  delinquencyRate: number;
  overdueCount: number;
  totalOverdue: number;
  par30: number;
  par60: number;
  par90: number;
}

export interface OverdueInstallment {
  installmentId: string;
  loanId: string;
  clientId: string;
  clientName: string;
  number: number;
  dueDate: string;
  daysOverdue: number;
  totalAmount: string;
  remainingAmount: string;
  status: string;
}

export interface KPIs {
  totalClients: number;
  activeLoans: number;
  totalDisbursed: number;
  delinquencyRate: number;
  collectionRate: number;
  averageLoanAmount: number;
}

export interface TrendData {
  month: string;
  amount: number;
  count: number;
}

// ---- Admin: Audit ----
export interface AuditEntry {
  id: string;
  userId?: string;
  action: string;
  entityType: string;
  entityId: string;
  description: string;
  ip: string;
  userAgent: string;
  createdAt: string;
}

// ---- Vendor ----
export interface Vendor {
  id: string;
  businessName: string;
  cuit: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
  isActive: boolean;
  email: string;
  createdAt: string;
}

export interface VendorAccount {
  id: string;
  vendorId: string;
  balance: string;
}

export interface VendorMovement {
  id: string;
  type: string;
  amount: string;
  balanceAfter: string;
  description: string;
  reference?: string;
  createdAt: string;
}

export interface Purchase {
  id: string;
  vendorId: string;
  vendorName?: string;
  clientId: string;
  clientName?: string;
  creditLineId: string;
  loanId: string;
  amount: string;
  description: string;
  createdAt: string;
}

export interface VendorPayment {
  id: string;
  vendorId: string;
  amount: string;
  method: string;
  reference?: string;
  paidBy: string;
  createdAt: string;
}

export interface RegisterVendorRequest {
  email: string;
  password: string;
  businessName: string;
  cuit: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
}

export interface UpdateVendorProfileRequest {
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
}

export interface RecordPurchaseRequest {
  clientId: string;
  creditLineId: string;
  amount: string;
  description: string;
  numInstallments: number;
  amortizationType: "french" | "german";
}

export interface RecordVendorPaymentRequest {
  vendorId: string;
  amount: string;
  method: string;
  reference?: string;
}

export interface RegisterClientByVendorRequest {
  email: string;
  firstName: string;
  lastName: string;
  dni: string;
  cuit: string;
  dateOfBirth: string;
  phone: string;
  address: string;
  city: string;
  province: string;
  country: string;
  isPEP: boolean;
}

export interface RequestCreditLineByVendorRequest {
  clientId: string;
  maxAmount: string;
  interestRate: string;
  maxInstallments: number;
}

// ---- Withdrawal Requests ----
export interface WithdrawalRequest {
  id: string;
  vendorId: string;
  vendorName?: string;
  amount: string;
  method: string;
  reference?: string;
  status: string;
  rejectionReason?: string;
  requestedAt: string;
  processedAt?: string;
  paymentId?: string;
}

export interface CreateWithdrawalRequest {
  amount: string;
  method: string;
}

export interface ApproveWithdrawalRequest {
  reference?: string;
}

export interface RejectWithdrawalRequest {
  reason: string;
}

// ---- Reports ----
export interface FinancialReport {
  interestAccrued: string;
  interestCollected: string;
  ivaAccrued: string;
  ivaCollected: string;
  capitalCollected: string;
  capitalPending: string;
}

export interface PortfolioPositionItem {
  status: string;
  loanCount: number;
  totalPrincipal: string;
  totalOutstanding: string;
}

export interface PortfolioPositionReport {
  items: PortfolioPositionItem[];
}

// ---- OTP ----
export interface RequestOTPRequest {
  email?: string;
  phone?: string;
  channel?: "email" | "phone";
}

export interface VerifyOTPRequest {
  email?: string;
  phone?: string;
  channel?: "email" | "phone";
  code: string;
}

// ---- Firebase Auth ----
// ---- Pagination ----
export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  offset: number;
  limit: number;
}

// ---- Credit risk ----
export type EmploymentType =
  | "employee"
  | "self_employed"
  | "business_owner"
  | "retired"
  | "informal"
  | "unemployed";

export type HousingStatus = "owned" | "mortgaged" | "rented" | "family" | "other";

export type RiskDecision = "approve" | "manual_review" | "decline";

export type RiskGrade = "A" | "B" | "C" | "D" | "E";

export type FactorImpact = "positive" | "negative" | "neutral";

export interface RiskProfile {
  id: string;
  clientId: string;
  clientName?: string;
  monthlyIncome: string;
  additionalIncome: string;
  grossMonthlyIncome: string;
  incomeVerified: boolean;
  employmentType: EmploymentType;
  employerName?: string;
  monthsEmployed: number;
  monthlyExpenses: string;
  existingDebtPayments: string;
  dependents: number;
  housingStatus: HousingStatus;
  hasBankAccount: boolean;
  bureauScore?: number;
  bureauScoreScale: number;
  adverseRecords: number;
  debtToIncomeRatio: string;
  expenseRatio: string;
  notes?: string;
  source: string;
  createdAt: string;
  updatedAt: string;
}

export interface SubmitRiskProfileRequest {
  monthlyIncome: string;
  additionalIncome?: string;
  incomeVerified: boolean;
  employmentType: EmploymentType;
  employerName?: string;
  monthsEmployed: number;
  monthlyExpenses?: string;
  existingDebtPayments?: string;
  dependents: number;
  housingStatus: HousingStatus;
  hasBankAccount: boolean;
  bureauScore?: number | null;
  adverseRecords: number;
  notes?: string;
}

export interface AdminSubmitRiskProfileRequest extends SubmitRiskProfileRequest {
  clientId: string;
}

export interface SimulateRiskRequest extends SubmitRiskProfileRequest {
  age: number;
}

export interface RiskFactor {
  code: string;
  label: string;
  value?: string;
  impact: FactorImpact;
  weight?: string;
  points: number;
  comment?: string;
}

export interface RiskAssessment {
  id: string;
  clientId: string;
  clientName?: string;
  riskProfileId: string;
  riskModelId: string;
  riskModelName: string;
  modelVersion: number;
  analyzer: string;
  score: number;
  grade: RiskGrade;
  probabilityOfDefault: string;
  decision: RiskDecision;
  maxMonthlyPayment: string;
  recommendedLimit: string;
  recommendedInstallments: number;
  recommendedInterestRate: string;
  summary?: string;
  policyNotes?: string[];
  factors?: RiskFactor[];
  createdAt: string;
}

export interface RiskEvaluation {
  score: number;
  grade: RiskGrade;
  probabilityOfDefault: string;
  decision: RiskDecision;
  maxMonthlyPayment: string;
  recommendedLimit: string;
  recommendedInstallments: number;
  recommendedInterestRate: string;
  summary?: string;
  policyNotes?: string[];
  factors?: RiskFactor[];
  analyzer: string;
}

export interface CreditAvailability {
  clientId: string;
  decision: RiskDecision;
  recommendedLimit: string;
  grantedLimit: string;
  usedAmount: string;
  availableAmount: string;
  recommendedInstallments: number;
  recommendedInterestRate: string;
  stale: boolean;
  assessment?: RiskAssessment;
}

export interface RiskGradeBand {
  grade: RiskGrade;
  minScore: number;
  interestSpread: string;
  exposureFactor: string;
  maxInstallments: number;
  decision: RiskDecision;
  probabilityOfDefault: string;
}

export interface RiskModelParams {
  minAge: number;
  maxAge: number;
  minMonthlyIncome: string;
  maxAdverseRecords: number;
  maxDti: string;
  maxPaymentToIncome: string;
  unverifiedIncomeHaircut: string;
  expensePerDependent: string;
  absoluteMaxLimit: string;
  maxInstallments: number;
  limitRoundingStep: string;
  baseInterestRate: string;
}

export interface RiskModel extends RiskModelParams {
  id: string;
  name: string;
  version: number;
  prompt: string;
  description?: string;
  isActive: boolean;
  bands: RiskGradeBand[];
  placeholders: string[];
  createdAt: string;
  updatedAt: string;
}

export interface SaveRiskModelVersionRequest {
  prompt: string;
  description?: string;
  params: RiskModelParams;
  bands?: RiskGradeBand[];
  activate: boolean;
}

export interface UpdateRiskModelRequest {
  prompt?: string;
  description?: string;
  params: RiskModelParams;
  bands?: RiskGradeBand[];
}

export interface PromptPreview {
  prompt: string;
  rendered: string;
  applicant: string;
  analyzer: string;
  placeholders: string[];
  warnings?: string[];
  baseline?: RiskEvaluation;
}

export interface RiskAnalyzerInfo {
  analyzer: string;
  promptDriven: boolean;
  placeholders: string[];
}

// ---- Admin: user management ----
export type UserRole = "admin" | "client" | "vendor" | "advisor";

/** A user account as returned by the admin user endpoints. */
export interface AdminUser {
  id: string;
  email: string;
  phone?: string;
  role: UserRole;
  isActive: boolean;
  lastLoginAt?: string;
  createdAt?: string;
}

/** A page of users plus the total before pagination. */
export interface UserListResponse {
  users: AdminUser[];
  total: number;
}

/** Filters accepted by the admin user listing. */
export interface AdminUserFilter {
  role?: UserRole;
  active?: boolean;
  search?: string;
  limit?: number;
  offset?: number;
}

export interface CreateUserRequest {
  email: string;
  phone?: string;
  role: UserRole;
}

// ---- Admin: livestock quota parameters ----

/** One typology's row of the UGG conversion matrix. */
export interface LivestockUGGFactor {
  typology: string;
  calves: string;
  growing: string;
  breeding: string;
  fattening: string;
  cull: string;
}

/**
 * Every assumption the livestock quota engine takes. Amounts, rates and factors
 * are decimal strings: money never travels as a float.
 */
export interface LivestockQuotaParams {
  priceCalfPerKg: string;
  priceGrowingPerKg: string;
  priceFatteningPerKg: string;
  weightCalfKg: string;
  weightGrowingKg: string;
  weightFatteningKg: string;

  milkingShare: string;
  pricePerLitre: string;

  vaccineCostPerUggYear: string;
  saltCostPerUggYear: string;
  supplementCostPerUggYear: string;
  vetCostPerUggYear: string;
  dailyWage: string;

  labourRegime: LivestockLabourRegime;
  uggThresholdFamily: string;
  uggThresholdOccasional: string;
  uggThresholdPartial: string;
  occasionalDaysPerMonth: number;

  fenceCostPerHaYear: string;
  infraCostPerUggYear: string;
  hectaresPerUgg: string;

  ltvPercent: string;
  annualRatePct: string;
  capitalCostPct: string;
  maxQuota: string;
  termMonths: number;
  applyInputCeiling: boolean;
  inputHeadroom: string;

  factors: LivestockUGGFactor[];
}

export type LivestockLabourRegime = "auto" | "familiar" | "formal";

/** One stored, versioned parameter set of the livestock quota engine. */
export interface LivestockQuotaPolicy extends LivestockQuotaParams {
  id: string;
  name: string;
  version: number;
  description?: string;
  isActive: boolean;
  /** The typologies the matrix must cover, in presentation order. */
  typologies: string[];
  createdAt: string;
  updatedAt: string;
}

export interface SaveLivestockQuotaPolicyVersionRequest {
  description?: string;
  params: LivestockQuotaParams;
  activate: boolean;
}

export interface UpdateLivestockQuotaPolicyRequest {
  description?: string;
  params: LivestockQuotaParams;
}

// ---- Admin: livestock quota lookup against Fedegán's vaccination registry ----

/** Identification documents the registry accepts, with the cédula as default. */
export type LivestockRegistryIdType =
  | "CÉDULA DE CIUDADANÍA"
  | "CÉDULA DE EXTRANJERÍA"
  | "NIT"
  | "PASAPORTE"
  | "LIBRETA MILITAR"
  | "TARJETA DE IDENTIDAD O REGISTRO CIVIL";

/** One predio as the registry reports it. */
export interface RegistryHolding {
  code: string;
  name: string;
  department: string;
  municipality: string;
  village?: string;
  cycle: number;
  year: number;
  vaccinatedAt?: string;
  head: number;
  notVaccinated: number;
}

/** Who the registry says the national ID belongs to. */
export interface RegistryRancher {
  nationalId: string;
  name: string;
  phone?: string;
  email?: string;
  holdings: RegistryHolding[];
}

/**
 * The four ceilings the quota is the minimum of. Decimal strings: money never
 * travels as a float.
 */
export interface LivestockQuotaCeilings {
  ltv: string;
  capacity: string;
  inputs: string;
  policyCap: string;
}

/** The 0-100 behaviour score and the three components it is built from. */
export interface LivestockScore {
  score: number;
  grade: string;
  vaccinationRatePct: number;
  cycleCompliancePct: number;
  seniorityCycles: number;
}

/** An assigned quota with the full trace of how it was reached. */
export interface LivestockQuotaAssessment {
  id: string;
  nationalId: string;
  reference: string;
  quota: string;
  currency: string;
  /**
   * The ceiling that bound the quota, as the engine names it in Spanish
   * ("LTV sobre flujo", "Capacidad de pago", "Necesidad de insumos",
   * "Tope de política"), not as a stable code.
   */
  activeCeiling: string;
  /** Stable key of activeCeiling: ltv, capacity, inputs or policyCap. */
  activeCeilingCode?: string;
  ceilings: LivestockQuotaCeilings;
  termMonths: number;
  periodRatePct: string;
  score: LivestockScore;
  typology: string;
  head: number;
  ugg: string;
  projectedIncome: string;
  eligibleBasket: string;
  createdAt: string;
}

/**
 * A quota computed from the registry. The assumptions are not decoration:
 * three of the score's inputs are absent from the registry, so whoever reads
 * the quota needs to know which figures were observed and which were assumed.
 */
export interface RegistryQuotaResponse {
  rancher: RegistryRancher;
  assessment: LivestockQuotaAssessment;
  assumptions: string[];
}
