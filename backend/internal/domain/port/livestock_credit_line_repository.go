package port

import (
	"context"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/google/uuid"
)

// LivestockCreditLineRepository stores ranchers' livestock credit lines and
// their signed promissory notes. The finders return nil, nil when nothing
// matches.
type LivestockCreditLineRepository interface {
	// Create stores the line, its document and the enrollment of the rancher
	// in Prestia's own tables (nil: none) together, or nothing.
	Create(ctx context.Context, line *model.LivestockCreditLine, doc *model.LivestockCreditLineDocument, enrollment *model.LivestockClientEnrollment) error
	// LinkClient stores the enrollment of a line registered without one and
	// points the line at its client and credit line, together or not at all.
	LinkClient(ctx context.Context, line *model.LivestockCreditLine, enrollment *model.LivestockClientEnrollment) error
	// CreateLoan stores a loan opened on the line, with its installments, the
	// credit line's new terms and usage, and the line's link to the loan,
	// together or not at all.
	CreateLoan(ctx context.Context, line *model.LivestockCreditLine, creditLine *model.CreditLine, loan *model.Loan) error
	FindByPromissoryNote(ctx context.Context, number string) (*model.LivestockCreditLine, error)
	FindByPartnerReference(ctx context.Context, partner, reference string) (*model.LivestockCreditLine, error)
	FindByAssessment(ctx context.Context, assessmentID uuid.UUID) (*model.LivestockCreditLine, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.LivestockCreditLine, error)
	// List returns a page of the lines matching the filter, most recently
	// activated first, and how many match in all. It never loads documents.
	List(ctx context.Context, filter LivestockCreditLineFilter) ([]model.LivestockCreditLine, int64, error)
	// FindDocument returns the signed promissory note of a line.
	FindDocument(ctx context.Context, creditLineID uuid.UUID) (*model.LivestockCreditLineDocument, error)
}

// LivestockCreditLineFilter selects credit lines for the back office. Empty
// fields do not filter.
type LivestockCreditLineFilter struct {
	// NationalID matches exactly.
	NationalID string
	Status     model.LivestockCreditLineStatus
	Offset     int
	Limit      int
}
