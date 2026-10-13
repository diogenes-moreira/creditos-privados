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
}

func NewLivestockQuotaService(repo port.LivestockQuotaRepository, currency string) *LivestockQuotaService {
	return &LivestockQuotaService{
		repo:     repo,
		yields:   model.DefaultLivestockYields(),
		currency: currency,
	}
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

// Assess computes and stores the quota. requestedBy names the partner service.
func (s *LivestockQuotaService) Assess(ctx context.Context, req LivestockQuotaRequest, requestedBy string) (*model.LivestockQuotaAssessment, model.LivestockAssessment, error) {
	applicant, err := toApplicant(req)
	if err != nil {
		return nil, model.LivestockAssessment{}, err
	}
	assessment := model.AssessQuota(applicant, s.params(ctx), s.yields)
	record, err := model.NewLivestockQuotaAssessment(applicant, assessment, s.currency, requestedBy, req.Reference)
	if err != nil {
		return nil, model.LivestockAssessment{}, err
	}
	if err := s.repo.Create(ctx, record); err != nil {
		return nil, model.LivestockAssessment{}, fmt.Errorf("storing livestock quota assessment: %w", err)
	}
	return record, assessment, nil
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
	Rancher    *port.RUVRancher
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
// from it. Tenure is assumed leased rather than owned, because over-granting
// credit is the more expensive mistake; every assumption is returned alongside
// the quota rather than buried.
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

	cycles := rancher.ReportedCycles()
	req := LivestockQuotaRequest{
		NationalID:    rancher.NationalID,
		Department:    rancher.Department(),
		Tenure:        model.TenureLeased,
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

	record, assessment, err := s.Assess(ctx, req, requestedBy)
	if err != nil {
		return nil, err
	}

	return &RegistryAssessment{
		Record:     record,
		Assessment: assessment,
		Rancher:    rancher,
		Assumptions: []string{
			"tenure_assumed_leased",
			"holding_status_assumed_active",
			"compliance_from_reported_cycles_only",
		},
	}, nil
}
