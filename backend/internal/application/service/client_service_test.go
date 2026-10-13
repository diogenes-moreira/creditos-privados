package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fakes embed the port so only the methods Register calls need bodies;
// anything else panics, which flags an unexpected call.

type fakeUsers struct {
	port.UserRepository
	byEmail map[string]*model.User
	created []*model.User
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*model.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeUsers) Create(_ context.Context, u *model.User) error {
	f.created = append(f.created, u)
	return nil
}

type fakeClients struct {
	port.ClientRepository
	created []*model.Client
}

func (f *fakeClients) FindByDNI(context.Context, string) (*model.Client, error) {
	return nil, errors.New("not found")
}

func (f *fakeClients) FindByCUIT(context.Context, string) (*model.Client, error) {
	return nil, errors.New("not found")
}

func (f *fakeClients) Create(_ context.Context, c *model.Client) error {
	f.created = append(f.created, c)
	return nil
}

type fakeAccounts struct {
	port.AccountRepository
	created int
}

func (f *fakeAccounts) Create(context.Context, *model.CurrentAccount) error {
	f.created++
	return nil
}

type fakeAuth struct {
	port.AuthService
	created int
}

func (f *fakeAuth) CreateUser(_ context.Context, email, _ string) (*port.AuthUser, error) {
	f.created++
	return &port.AuthUser{UID: "uid-" + email, Email: email}, nil
}

type fakeAudit struct {
	port.AuditLogRepository
	logs []*model.AuditLog
}

func (f *fakeAudit) Create(_ context.Context, l *model.AuditLog) error {
	f.logs = append(f.logs, l)
	return nil
}

func newClientServiceWithAudit(users *fakeUsers, clients *fakeClients, accounts *fakeAccounts, auth *fakeAuth, audit *fakeAudit) *service.ClientService {
	return service.NewClientService(users, clients, accounts, auth, service.NewAuditService(audit), "CO")
}

func newClientService(users *fakeUsers, clients *fakeClients, accounts *fakeAccounts, auth *fakeAuth) *service.ClientService {
	return newClientServiceWithAudit(users, clients, accounts, auth, &fakeAudit{})
}

func TestRegisterRejectsAnInvalidClientWithoutCreatingTheAccount(t *testing.T) {
	users, clients, accounts, auth := &fakeUsers{}, &fakeClients{}, &fakeAccounts{}, &fakeAuth{}
	s := newClientService(users, clients, accounts, auth)

	// The NIT's check digit is wrong: 17788090's is 7.
	_, _, err := s.Register(context.Background(), nil, "ganadero@example.com", "Juan", "Pérez",
		"17788090", "17788090-1", "1970-05-01", "+573000000000", "Predio La Esperanza", "Popayán", "Cauca", "Colombia", false)

	require.Error(t, err)
	assert.Empty(t, users.created, "a rejected client must not leave an account behind")
	assert.Zero(t, auth.created)
	assert.Empty(t, clients.created)
	assert.Zero(t, accounts.created)
}

func TestRegisterRefusesAnEmailAlreadyInUse(t *testing.T) {
	users := &fakeUsers{byEmail: map[string]*model.User{"ganadero@example.com": {Email: "ganadero@example.com"}}}
	clients, accounts, auth := &fakeClients{}, &fakeAccounts{}, &fakeAuth{}
	s := newClientService(users, clients, accounts, auth)

	_, _, err := s.Register(context.Background(), nil, "ganadero@example.com", "Juan", "Pérez",
		"17788090", "17788090-7", "1970-05-01", "+573000000000", "Predio La Esperanza", "Popayán", "Cauca", "Colombia", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Empty(t, users.created)
}

func TestRegisterLinksTheClientToTheAccountItCreates(t *testing.T) {
	users, clients, accounts, auth := &fakeUsers{}, &fakeClients{}, &fakeAccounts{}, &fakeAuth{}
	s := newClientService(users, clients, accounts, auth)

	// A natural person's NIT is the cédula plus its check digit.
	client, user, err := s.Register(context.Background(), nil, "ganadero@example.com", "Juan", "Pérez",
		"17788090", "17788090-7", "1970-05-01", "+573000000000", "Predio La Esperanza", "Popayán", "Cauca", "Colombia", false)

	require.NoError(t, err)
	require.Len(t, users.created, 1)
	require.Len(t, clients.created, 1)
	assert.Equal(t, user.ID, client.UserID)
	assert.Equal(t, model.RoleClient, user.Role)
	assert.Equal(t, 1, accounts.created)
}

func TestRegisterOnSomeonesBehalfIsAuditedToWhoeverDidIt(t *testing.T) {
	audit := &fakeAudit{}
	s := newClientServiceWithAudit(&fakeUsers{}, &fakeClients{}, &fakeAccounts{}, &fakeAuth{}, audit)
	advisor := uuid.New()

	_, user, err := s.Register(context.Background(), &advisor, "ganadero@example.com", "Juan", "Pérez",
		"17788090", "17788090-7", "1970-05-01", "+573000000000", "Predio La Esperanza", "Popayán", "Cauca", "Colombia", false)

	require.NoError(t, err)
	require.Len(t, audit.logs, 1)
	require.NotNil(t, audit.logs[0].UserID)
	assert.Equal(t, advisor, *audit.logs[0].UserID, "the advisor, not the new client")
	assert.NotEqual(t, user.ID, *audit.logs[0].UserID)
	assert.Contains(t, audit.logs[0].Description, "on their behalf")
}

func TestSelfRegistrationIsAuditedToTheNewClient(t *testing.T) {
	audit := &fakeAudit{}
	s := newClientServiceWithAudit(&fakeUsers{}, &fakeClients{}, &fakeAccounts{}, &fakeAuth{}, audit)

	_, user, err := s.Register(context.Background(), nil, "ganadero@example.com", "Juan", "Pérez",
		"17788090", "17788090-7", "1970-05-01", "+573000000000", "Predio La Esperanza", "Popayán", "Cauca", "Colombia", false)

	require.NoError(t, err)
	require.Len(t, audit.logs, 1)
	assert.Equal(t, user.ID, *audit.logs[0].UserID)
}
