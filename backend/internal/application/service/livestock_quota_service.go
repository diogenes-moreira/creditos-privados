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
