package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// LivestockQuotaRequest is a rancher's RUV data as a partner service sends it:
// one herd per holding (predio), summed because the quota is one per national
// ID, plus the RUV behaviour history the score needs.
type LivestockQuotaRequest struct {
	NationalID      string
	Department      string
	Holdings        []model.Herd
	Tenure          string
	HoldingStatus   string
	SeniorityCycles int
	CompletedCycles int
	ReportedCycles  int
	// Reference is the caller's identifier for the request.
	Reference string
}

// ErrInvalidLivestockQuotaRequest wraps every validation failure, so the HTTP
// layer can answer 400 instead of 500.
var ErrInvalidLivestockQuotaRequest = errors.New("invalid livestock quota request")

// LivestockQuotaService runs the livestock quota engine for partner services
// and stores every result.
type LivestockQuotaService struct {
	repo port.LivestockQuotaRepository
	// policies supplies the operator-edited parameters. It is optional: with no
	// policy service wired in, the engine runs on the shipped defaults.
	policies *LivestockQuotaPolicyService
	// registry reads herds from the RUV. Optional: without it the service only
	// scores herds a caller supplies.
	registry port.RUVClient
	yields   model.LivestockYields
	currency string

	// strategy decides which figure is granted. The engine runs under both,
	// so its quota is always on record for comparison.
	strategy model.QuotaStrategy
	tiers    model.QuotaTierPolicy
	// currentCycle is the RUV cycle the tiers measure the herd on.
	currentCycle model.VaccinationCycle
}

// NewLivestockQuotaService builds the service on the tiers strategy, the
// pilot's default, measured on model.DefaultCurrentCycle.
func NewLivestockQuotaService(repo port.LivestockQuotaRepository, currency string) *LivestockQuotaService {
	return &LivestockQuotaService{
		repo:         repo,
		yields:       model.DefaultLivestockYields(),
		currency:     currency,
		strategy:     model.QuotaStrategyTiers,
		tiers:        model.DefaultQuotaTierPolicy(),
		currentCycle: model.DefaultCurrentCycle,
	}
}

// WithStrategy selects how the quota is decided.
func (s *LivestockQuotaService) WithStrategy(strategy model.QuotaStrategy) *LivestockQuotaService {
	s.strategy = strategy
	return s
}

// WithCurrentCycle sets the RUV cycle the tiers are measured on.
func (s *LivestockQuotaService) WithCurrentCycle(c model.VaccinationCycle) *LivestockQuotaService {
	s.currentCycle = c
	return s
}

// Strategy reports how the quota is decided.
func (s *LivestockQuotaService) Strategy() model.QuotaStrategy { return s.strategy }

// LivestockQuotaOutcome is one stored quota: the record, the engine's full
// result and, under the tiers strategy, the tier decision that set the quota.
type LivestockQuotaOutcome struct {
	Record     *model.LivestockQuotaAssessment
	Assessment model.LivestockAssessment
	// Tier is nil under the engine strategy.
	Tier *model.TierAssessment
}

// WithPolicies makes the engine run on the active stored parameters.
func (s *LivestockQuotaService) WithPolicies(policies *LivestockQuotaPolicyService) *LivestockQuotaService {
	s.policies = policies
	return s
}

// params resolves the parameters for one assessment: the active policy's when
// one is stored, the shipped defaults otherwise.
func (s *LivestockQuotaService) params(ctx context.Context) model.QuotaParams {
	if s.policies == nil {
		return model.DefaultQuotaParams()
	}
	return s.policies.ActiveParams(ctx)
}

// Assess computes and stores the quota for a herd the caller supplies.
// requestedBy names the partner service.
//
// Under the tiers strategy the supplied herd is taken as the current cycle,
// since it carries no cycle, and the caller's own tenure and holding status
// are used. No document type travels with it, so the cédula filter is skipped.
func (s *LivestockQuotaService) Assess(ctx context.Context, req LivestockQuotaRequest, requestedBy string) (*LivestockQuotaOutcome, error) {
	return s.assess(ctx, req, requestedBy, func(applicant model.LivestockApplicant) model.TierEvidence {
		c := applicant.Herd.Classify()
		return model.TierEvidence{
			CurrentCycle:           s.currentCycle,
			CurrentCycleRegistered: true,
			VaccinatedHeads:        c.Total,
			NotVaccinated:          c.NotVaccinated,
			HoldingActive:          applicant.HoldingStatus == model.HoldingActive,
			OwnedHolding:           applicant.Tenure == model.TenureOwned,
		}
	})
}

// assess runs the engine, applies the tiers when they are the strategy, and
// stores the result. evidence measures the tiers' inputs for the applicant.
func (s *LivestockQuotaService) assess(ctx context.Context, req LivestockQuotaRequest, requestedBy string,
	evidence func(model.LivestockApplicant) model.TierEvidence) (*LivestockQuotaOutcome, error) {
	applicant, err := toApplicant(req)
	if err != nil {
		return nil, err
	}
	assessment := model.AssessQuota(applicant, s.params(ctx), s.yields)
	record, err := model.NewLivestockQuotaAssessment(applicant, assessment, s.currency, requestedBy, req.Reference)
	if err != nil {
		return nil, err
	}
	outcome := &LivestockQuotaOutcome{Record: record, Assessment: assessment}
	if s.strategy == model.QuotaStrategyTiers {
		tier := s.tiers.Assess(evidence(applicant))
		if err := record.ApplyTier(assessment, tier); err != nil {
			return nil, err
		}
		outcome.Tier = &tier
	}
	if err := s.repo.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("storing livestock quota assessment: %w", err)
	}
	return outcome, nil
}

func toApplicant(req LivestockQuotaRequest) (model.LivestockApplicant, error) {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidLivestockQuotaRequest, fmt.Sprintf(format, args...))
	}
	if strings.TrimSpace(req.NationalID) == "" {
		return model.LivestockApplicant{}, invalid("national ID is required")
	}
	if len(req.Holdings) == 0 {
		return model.LivestockApplicant{}, invalid("at least one holding is required")
	}
	if req.CompletedCycles > req.ReportedCycles {
		return model.LivestockApplicant{}, invalid("completed cycles (%d) exceed reported cycles (%d)",
			req.CompletedCycles, req.ReportedCycles)
	}
	var herd model.Herd
	for _, h := range req.Holdings {
		herd.Add(h)
	}
	return model.LivestockApplicant{
		NationalID:      strings.TrimSpace(req.NationalID),
		Department:      strings.TrimSpace(req.Department),
		Herd:            herd,
		Tenure:          req.Tenure,
		HoldingStatus:   req.HoldingStatus,
		SeniorityCycles: req.SeniorityCycles,
		CompletedCycles: req.CompletedCycles,
		ReportedCycles:  req.ReportedCycles,
	}, nil
}

// WithRegistry lets the service read herds from the RUV itself.
func (s *LivestockQuotaService) WithRegistry(registry port.RUVClient) *LivestockQuotaService {
	s.registry = registry
	return s
}

// ErrNotInRegistry means the RUV holds no record for that national ID. It is a
// normal outcome, not a failure: most of the target population is not
// registered yet.
var ErrNotInRegistry = errors.New("no RUV record for that national ID")

// ErrRegistryUnavailable means the deployment has no registry configured.
var ErrRegistryUnavailable = errors.New("the RUV is not configured")

// RegistryAssessment is a quota computed from the registry, carrying what had
// to be assumed because the RUV does not report it.
type RegistryAssessment struct {
	Record     *model.LivestockQuotaAssessment
	Assessment model.LivestockAssessment
	// Tier is the tier decision; nil under the engine strategy.
	Tier    *model.TierAssessment
	Rancher *port.RUVRancher
	// Assumptions names every score input that was not observed. The score
	// moves on these, so whoever reads the quota should see them.
	Assumptions []string
}

// AssessFromRegistry reads the rancher's herds from the RUV and scores them.
//
// Three of the score's inputs are not in the registry: whether the land is
// owned or leased (10 points), the holding's status (5 points), and the
// compliance history (30 points). A single query returns the cycles that were
// reported, never the ones that were missed, so compliance cannot be derived
// from it. Every assumption is returned alongside the quota rather than buried.
//
// Tenure, for the engine's score and the tiers alike, comes from the records'
// "propietario" (port.RUVRancher.OwnsAHolding): owned only when it names the
// rancher. Empty, as the RUV sends it today, counts as leased / not owned
// (model.RUVHoldingOwned), because over-granting credit is the more expensive
// mistake; the top tier stays out of reach until FEDEGÁN fills it.
//
// Under the tiers strategy the herd is measured on the current cycle only and
// the holding is taken as active. A rancher found in the RUV who reaches no
// tier is still a 200 with a zero quota; only an ID the RUV does not know is
// ErrNotInRegistry.
//
// reference is the caller's identifier for the request (e.g. CuppoGan's
// onboarding ID); empty means "ruv".
func (s *LivestockQuotaService) AssessFromRegistry(ctx context.Context, idType, nationalID, requestedBy, reference string) (*RegistryAssessment, error) {
	if s.registry == nil {
		return nil, ErrRegistryUnavailable
	}
	rancher, err := s.registry.FindByNationalID(ctx, idType, nationalID)
	if err != nil {
		return nil, fmt.Errorf("reading the RUV: %w", err)
	}
	if rancher == nil {
		return nil, ErrNotInRegistry
	}

	owned := rancher.OwnsAHolding() || model.RUVHoldingOwned
	tenure := model.TenureLeased
	if owned {
		tenure = model.TenureOwned
	}

	cycles := rancher.ReportedCycles()
	req := LivestockQuotaRequest{
		NationalID:    rancher.NationalID,
		Department:    rancher.Department(),
		Tenure:        tenure,
		HoldingStatus: model.HoldingActive,
		// Only reported cycles are visible, so compliance is 100% by
		// construction and seniority is capped by what one query shows.
		SeniorityCycles: cycles,
		CompletedCycles: cycles,
		ReportedCycles:  cycles,
		Reference:       reference,
	}
	if strings.TrimSpace(req.Reference) == "" {
		req.Reference = "ruv"
	}
	for _, h := range rancher.LatestHoldings() {
		req.Holdings = append(req.Holdings, h.Herd)
	}

	outcome, err := s.assess(ctx, req, requestedBy, func(model.LivestockApplicant) model.TierEvidence {
		herd, registered := rancher.CycleHerd(s.currentCycle)
		c := herd.Classify()
		return model.TierEvidence{
			IDTypeKnown: true,
			// The registry is queried by cédula when no type is given.
			IsCedula:               strings.TrimSpace(idType) == "" || strings.EqualFold(strings.TrimSpace(idType), port.RUVIDTypeCedula),
			CurrentCycle:           s.currentCycle,
			CurrentCycleRegistered: registered,
			VaccinatedHeads:        c.Total,
			NotVaccinated:          c.NotVaccinated,
			HoldingActive:          true,
			OwnedHolding:           owned,
		}
	})
	if err != nil {
		return nil, err
	}

	assumptions := []string{
		// Owned only when "propietario" says so; empty counts as not owned.
		"tenure_from_ruv_propietario",
		"holding_status_assumed_active",
		"compliance_from_reported_cycles_only",
	}
	return &RegistryAssessment{
		Record:      outcome.Record,
		Assessment:  outcome.Assessment,
		Tier:        outcome.Tier,
		Rancher:     rancher,
		Assumptions: assumptions,
	}, nil
}
