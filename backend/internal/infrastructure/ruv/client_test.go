package ruv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerTextAcceptsAnyJSONType(t *testing.T) {
	for raw, want := range map[string]string{
		`null`:           "",
		`""`:             "",
		`"  Sí "`:        "Sí",
		`"17.788.090"`:   "17.788.090",
		`true`:           "true",
		`false`:          "false",
		`17788090`:       "17788090",
		`{"nombre":"x"}`: "",
		`["si"]`:         "",
	} {
		assert.Equal(t, want, ownerText([]byte(raw)), raw)
	}
	assert.Equal(t, "", ownerText(nil), "absent")
}

// TestFindByNationalIDDecodesPropietario serves the registry's two calls (token
// and scoring) and checks that any propietario type decodes without failing.
func TestFindByNationalIDDecodesPropietario(t *testing.T) {
	records := []string{`null`, `true`, `"JOSÉ PÉREZ"`, `17788090`, `{"x":1}`}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.URL.Path, "getRuvScoring") {
			_, _ = fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
			return
		}
		var data []string
		for i, p := range records {
			data = append(data, fmt.Sprintf(`{"codigo_ruv":"%d","ganadero":"JOSE PEREZ","identificacion_ganadero":"17788090","ciclo":"1","ano":"2026","propietario":%s}`, i, p))
		}
		_, _ = fmt.Fprintf(w, `{"status":"ok","data":[%s]}`, strings.Join(data, ","))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ClientID: "id", ClientSecret: "secret", Username: "u", Password: "p"})
	require.NotNil(t, c)
	r, err := c.FindByNationalID(context.Background(), "", "17788090")
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Len(t, r.Holdings, len(records))

	var owners []string
	for _, h := range r.Holdings {
		owners = append(owners, h.Owner)
		assert.Equal(t, "JOSE PEREZ", h.Registrant)
		assert.Equal(t, "17788090", h.RegistrantID)
	}
	assert.Equal(t, []string{"", "true", "JOSÉ PÉREZ", "17788090", ""}, owners)
	assert.True(t, r.OwnsAHolding())
}

func TestFindByNationalIDKeepsTheFirstContactGivenInAnyRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.URL.Path, "getRuvScoring") {
			_, _ = fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"ok","data":[
			{"codigo_ruv":"1","ganadero":"JOSE PEREZ","correo_ganadero":" ","celular_ganadero":"","ciclo":"1","ano":"2026"},
			{"codigo_ruv":"2","ganadero":"JOSE PEREZ","correo_ganadero":"jose@example.com","celular_ganadero":"3000000000","ciclo":"1","ano":"2026"},
			{"codigo_ruv":"3","ganadero":"JOSE PEREZ","correo_ganadero":"otro@example.com","ciclo":"1","ano":"2026"}]}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ClientID: "id", ClientSecret: "secret", Username: "u", Password: "p"})
	r, err := c.FindByNationalID(context.Background(), "", "17788090")
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, "jose@example.com", r.Email, "a predio without e-mail does not hide the others'")
	assert.Equal(t, "3000000000", r.Phone)
	assert.Equal(t, "JOSE PEREZ", r.Name)
}
