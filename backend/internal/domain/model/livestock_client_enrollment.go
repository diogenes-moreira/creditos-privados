package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/diogenes-moreira/creditos/backend/pkg/validator"
)

// LivestockClientEnrollment is what a livestock credit activation adds to
// Prestia's own tables: the rancher as a Client, with its User and current
// account, when Prestia does not know them yet, and the approved CreditLine of
// the quota. The CuppoGan specifics (assessment, promissory note, Odoo link)
// stay in LivestockCreditLine, which points at both.
type LivestockClientEnrollment struct {
	// User and Account are nil when the client already existed.
	User       *User
	Account    *CurrentAccount
	Client     *Client
	CreditLine *CreditLine
}

// NewClient tells whether the enrollment creates the client.
func (e *LivestockClientEnrollment) NewClient() bool { return e.User != nil }

// GenericDateOfBirth stands in for a rancher whose date of birth the partner
// does not know (Prestia requires one, of an adult). The client's comments say
// so.
var GenericDateOfBirth = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)

// GenericDateOfBirthComment is the client comment that flags it.
const GenericDateOfBirthComment = "Fecha de nacimiento genérica: el aliado no la informó."

// RegistryEmailDomain makes the e-mail of a rancher without one (or whose
// e-mail another user already has): <national ID>@cuppogan.com. Prestia
// requires a unique e-mail per user.
const RegistryEmailDomain = "cuppogan.com"

var plausibleEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// ClientNames are the first and last names for the client: the ones sent, or
// FullName split at its first space.
func (a LivestockCreditActivation) ClientNames() (first, last string) {
	first, last = strings.TrimSpace(a.FirstName), strings.TrimSpace(a.LastName)
	if first != "" && last != "" {
		return first, last
	}
	parts := strings.Fields(a.FullName)
	switch len(parts) {
	case 0:
		return first, last
	case 1:
		return parts[0], parts[0]
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

// ClientDateOfBirth is the date of birth sent, or GenericDateOfBirth (and
// generic true) when there is none.
func (a LivestockCreditActivation) ClientDateOfBirth() (dob time.Time, generic bool) {
	if a.DateOfBirth == nil || a.DateOfBirth.IsZero() {
		return GenericDateOfBirth, true
	}
	return *a.DateOfBirth, false
}

// ClientEmail is the e-mail sent, lowercased, when it looks like one; empty
// otherwise.
func (a LivestockCreditActivation) ClientEmail() string {
	email := strings.ToLower(strings.TrimSpace(a.Email))
	if !plausibleEmail.MatchString(email) {
		return ""
	}
	return email
}

// RegistryEmail is <national ID>@cuppogan.com.
func (a LivestockCreditActivation) RegistryEmail() string {
	return strings.ToLower(strings.TrimSpace(a.NationalID)) + "@" + RegistryEmailDomain
}

// NaturalPersonNIT is the NIT of a natural person: the cédula followed by its
// check digit ("12345678-9").
func NaturalPersonNIT(cedula string) (string, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(cedula), ".", "")
	if err := validator.ValidateCedula(cleaned); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d", cleaned, validator.CalculateNITCheckDigit(cleaned)), nil
}
