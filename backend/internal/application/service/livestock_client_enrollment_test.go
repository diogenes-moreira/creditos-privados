package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enrollClients and enrollUsers implement only what the enrollment reads.
type enrollClients struct {
	port.ClientRepository
	byDNI map[string]*model.Client
}

func (f *enrollClients) FindByDNI(_ context.Context, dni string) (*model.Client, error) {
	if c, ok := f.byDNI[dni]; ok {
		return c, nil
	}
	return nil, errors.New("client not found: record not found")
}

type enrollUsers struct {
	port.UserRepository
	emails map[string]bool
}

func (f *enrollUsers) FindByEmail(_ context.Context, email string) (*model.User, error) {
	if f.emails[email] {
		return &model.User{ID: uuid.New(), Email: email}, nil
	}
	return nil, errors.New("user not found: record not found")
}

type enrollAuth struct{ port.AuthService }

func (enrollAuth) CreateUser(_ context.Context, email, _ string) (*port.AuthUser, error) {
	return &port.AuthUser{UID: uuid.NewString(), Email: email}, nil
}

var terms = service.LivestockCreditTerms{InterestRate: decimal.RequireFromString("2.5"), MaxInstallments: 6}

func enrollingFixture() (*service.LivestockCreditLineService, *fakeCreditLines, *enrollClients, *enrollUsers, model.LivestockCreditActivation) {
	s, lines, a := creditFixture()
	clients := &enrollClients{byDNI: map[string]*model.Client{}}
	users := &enrollUsers{emails: map[string]bool{}}
	s.WithClients(users, clients, enrollAuth{}, "CO", terms)
	a.MobilePhone = "+573001234567"
	return s, lines, clients, users, a
}

func TestActivationEnrollsTheRancherAsAClientWithAnApprovedCreditLine(t *testing.T) {
	s, lines, _, _, a := enrollingFixture()
	dob := time.Date(1975, 4, 30, 0, 0, 0, 0, time.UTC)
	a.FirstName, a.LastName, a.Email, a.DateOfBirth = "Ganadero", "De Prueba", "Ganadero@Example.com", &dob
	a.Address, a.City, a.Province = "Vereda El Cairo, predio La Esperanza", "MONTERIA", "CORDOBA"

	line, created, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	require.NoError(t, err)
	assert.True(t, created)
	require.Len(t, lines.enrollments, 1)
	e := lines.enrollments[0]
	require.True(t, e.NewClient())
	assert.Equal(t, "ganadero@example.com", e.User.Email)
	assert.Equal(t, model.RoleClient, e.User.Role)
	assert.Equal(t, "+573001234567", e.User.Phone)
	assert.Equal(t, e.User.ID, e.Client.UserID)
	assert.Equal(t, "Ganadero", e.Client.FirstName)
	assert.Equal(t, "De Prueba", e.Client.LastName)
	assert.Equal(t, "17788090", e.Client.DNI)
	assert.Equal(t, "17788090-7", e.Client.CUIT, "NIT: the cédula with its check digit")
	assert.Equal(t, dob, e.Client.DateOfBirth)
	assert.Empty(t, e.Client.Comments)
	assert.Equal(t, "MONTERIA", e.Client.City)
	assert.Equal(t, "CORDOBA", e.Client.Province)
	assert.Equal(t, "Colombia", e.Client.Country)
	assert.Equal(t, e.Client.ID, e.Account.ClientID)

	cl := e.CreditLine
	assert.Equal(t, e.Client.ID, cl.ClientID)
	assert.Equal(t, model.CreditLineApproved, cl.Status)
	assert.Equal(t, "1000000.00", cl.MaxAmount.StringFixed(2))
	assert.True(t, cl.UsedAmount.IsZero())
	assert.Equal(t, "2.5", cl.InterestRate.String())
	assert.Equal(t, 6, cl.MaxInstallments)
	require.NotNil(t, cl.ApprovedAt)
	assert.Equal(t, a.ActivatedAt.UTC(), *cl.ApprovedAt)

	assert.Equal(t, e.Client.ID, *line.ClientID)
	assert.Equal(t, cl.ID, *line.CreditLineID)
}

func TestActivationWithoutClientDataUsesTheFallbacks(t *testing.T) {
	s, lines, _, _, a := enrollingFixture()

	_, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	require.NoError(t, err)
	e := lines.enrollments[0]
	assert.Equal(t, "17788090@cuppogan.com", e.User.Email)
	assert.Equal(t, "GANADERO", e.Client.FirstName, "FullName split at its first space")
	assert.Equal(t, "DE PRUEBA", e.Client.LastName)
	assert.Equal(t, model.GenericDateOfBirth, e.Client.DateOfBirth)
	assert.Equal(t, model.GenericDateOfBirthComment, e.Client.Comments)
}

func TestAnEmailAnotherUserHasGivesWayToTheNationalIDOne(t *testing.T) {
	s, lines, _, users, a := enrollingFixture()
	a.Email = "pendiente@cuppogan.com"
	users.emails["pendiente@cuppogan.com"] = true

	_, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	require.NoError(t, err)
	assert.Equal(t, "17788090@cuppogan.com", lines.enrollments[0].User.Email)
}

func TestAKnownClientOnlyGetsTheNewCreditLine(t *testing.T) {
	s, lines, clients, _, a := enrollingFixture()
	known := &model.Client{ID: uuid.New(), DNI: "17788090", FirstName: "Ya", LastName: "Cargado"}
	clients.byDNI["17788090"] = known

	line, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	require.NoError(t, err)
	e := lines.enrollments[0]
	assert.False(t, e.NewClient())
	assert.Nil(t, e.Account)
	assert.Same(t, known, e.Client)
	assert.Equal(t, known.ID, e.CreditLine.ClientID)
	assert.Equal(t, known.ID, *line.ClientID)
}

func TestReplayingTheActivationOfALineWithoutClientEnrollsTheRancherOnce(t *testing.T) {
	s, lines, a := creditFixture()
	a.MobilePhone = "+573001234567"
	legacy, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)
	require.Nil(t, legacy.ClientID, "registered before activations enrolled ranchers")

	s.WithClients(&enrollUsers{emails: map[string]bool{}}, &enrollClients{byDNI: map[string]*model.Client{}}, enrollAuth{}, "CO", terms)
	line, created, err := s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Same(t, legacy, line)
	require.Len(t, lines.enrollments, 1)
	assert.Equal(t, lines.enrollments[0].Client.ID, *line.ClientID)

	_, _, err = s.RegisterActivation(context.Background(), a, "cuppogan")
	require.NoError(t, err)
	assert.Len(t, lines.enrollments, 1, "an enrolled line is not enrolled again")
}

func TestAClientPrestiaCannotAcceptIsRejectedBeforeStoringAnything(t *testing.T) {
	s, lines, _, _, a := enrollingFixture()
	a.MobilePhone = ""

	_, _, err := s.RegisterActivation(context.Background(), a, "cuppogan")

	assert.Equal(t, "invalid_client", code(t, err))
	assert.Empty(t, lines.lines)
}

func TestNaturalPersonNIT(t *testing.T) {
	nit, err := model.NaturalPersonNIT("17.788.090")
	require.NoError(t, err)
	assert.Equal(t, "17788090-7", nit)
	_, err = model.NaturalPersonNIT("12AB")
	assert.Error(t, err)
}
