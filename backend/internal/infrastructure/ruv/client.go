// Package ruv reads Fedegán's Registro Único de Vacunación.
package ruv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// Identification types the registry accepts.
const (
	IDTypeCedula      = "CÉDULA DE CIUDADANÍA"
	IDTypeExtranjeria = "CÉDULA DE EXTRANJERÍA"
	IDTypeNIT         = "NIT"
	IDTypePasaporte   = "PASAPORTE"
)

// Config holds the registry endpoint and OAuth2 credentials.
//
// The service is published over plain HTTP on a bare address, so the
// credentials and the personal data in the response travel unencrypted. That is
// the provider's contract, not a choice here; it is worth raising with Fedegán
// before this carries production volume.
type Config struct {
	// BaseURL is the service root, e.g. http://190.145.9.75:8185
	BaseURL      string
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
}

// Enabled reports whether enough is configured to query the registry.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.BaseURL) != "" &&
		strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.Username) != ""
}

// Client queries the RUV scoring service.
type Client struct {
	cfg    Config
	client *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

// New builds a client, or returns nil when the registry is not configured so
// the caller can fall back to manually entered herd data.
func New(cfg Config) *Client {
	if !cfg.Enabled() {
		return nil
	}
	return &Client{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// accessToken returns a cached token, refreshing it shortly before expiry.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", c.cfg.Username)
	form.Set("password", c.cfg.Password)
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)

	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/oauth/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("ruv: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ruv: requesting token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Never echo the credentials in the error.
		return "", fmt.Errorf("ruv: token endpoint returned %s", resp.Status)
	}

	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("ruv: decoding token: %w", err)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("ruv: token endpoint returned no access_token")
	}

	ttl := time.Duration(body.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	// Refresh a minute early so a request never races the expiry.
	c.token = body.AccessToken
	c.tokenExpiry = time.Now().Add(ttl - time.Minute)
	return c.token, nil
}

// scoringRecord is one predio as the service returns it. Counts arrive as JSON
// numbers; json.Number keeps them exact regardless of how they are encoded.
type scoringRecord struct {
	CodigoRUV       string `json:"codigo_ruv"`
	NombrePredio    string `json:"nombre_predio"`
	Departamento    string `json:"departamento"`
	Municipio       string `json:"municipio"`
	Vereda          string `json:"vereda"`
	Ciclo           string `json:"ciclo"`
	Ano             string `json:"ano"`
	Ganadero        string `json:"ganadero"`
	Identificacion  string `json:"identificacion_ganadero"`
	Celular         string `json:"celular_ganadero"`
	Correo          string `json:"correo_ganadero"`
	FechaVacunacion string `json:"fecha_vacunacion"`

	FemalesUnder3 json.Number `json:"aftosa_bov_h_0_3_m"`
	Females3To8   json.Number `json:"aftosa_bov_h_3_8_m"`
	Females8To12  json.Number `json:"aftosa_bov_h_8_12_m"`
	Females1To2   json.Number `json:"aftosa_bov_h_1_2_a"`
	Females2To3   json.Number `json:"aftosa_bov_h_2_3_a"`
	Females3To5   json.Number `json:"aftosa_bov_h_3_5_a"`
	FemalesOver5  json.Number `json:"aftosa_bov_h_5_x_a"`
	MalesUnder3   json.Number `json:"aftosa_bov_m_0_3_m"`
	Males3To8     json.Number `json:"aftosa_bov_m_3_8_m"`
	Males8To12    json.Number `json:"aftosa_bov_m_8_12_m"`
	Males1To2     json.Number `json:"aftosa_bov_m_1_2_a"`
	Males2To3     json.Number `json:"aftosa_bov_m_2_3_a"`
	MalesOver3    json.Number `json:"aftosa_bov_m_3_x_a"`
	NotVaccinated json.Number `json:"total_aftosa_bovinos_nv"`
}

func count(n json.Number) int {
	if n == "" {
		return 0
	}
	v, err := n.Int64()
	if err != nil {
		// A float or malformed count is treated as absent rather than
		// failing the whole evaluation over one column.
		f, ferr := n.Float64()
		if ferr != nil {
			return 0
		}
		return int(f)
	}
	return int(v)
}

func atoi(s string) int {
	var v int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &v)
	if err != nil {
		return 0
	}
	return v
}

// herd maps the registry's age and sex columns onto the domain's herd.
//
// CalvesUnder1Year stays zero: animals from 0 to 3 months already arrive in
// the 0_3_m columns, and the domain field only exists because the reference
// prototype was built on synthetic data that carried it separately.
func (r scoringRecord) herd() model.Herd {
	return model.Herd{
		FemalesUnder3Months: count(r.FemalesUnder3),
		Females3To8Months:   count(r.Females3To8),
		Females8To12Months:  count(r.Females8To12),
		Females1To2Years:    count(r.Females1To2),
		Females2To3Years:    count(r.Females2To3),
		Females3To5Years:    count(r.Females3To5),
		FemalesOver5Years:   count(r.FemalesOver5),
		MalesUnder3Months:   count(r.MalesUnder3),
		Males3To8Months:     count(r.Males3To8),
		Males8To12Months:    count(r.Males8To12),
		Males1To2Years:      count(r.Males1To2),
		Males2To3Years:      count(r.Males2To3),
		MalesOver3Years:     count(r.MalesOver3),
		NotVaccinated:       count(r.NotVaccinated),
	}
}

// FindByNationalID queries the registry. It returns nil when the registry has
// no record, which is a valid answer rather than an error: most of the target
// population is simply not in it yet.
func (c *Client) FindByNationalID(ctx context.Context, idType, id string) (*port.RUVRancher, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(idType) == "" {
		idType = IDTypeCedula
	}

	endpoint := fmt.Sprintf("%s/ruv/getRuvScoring?identificacion=%s&tipoIdentificacion=%s",
		strings.TrimRight(c.cfg.BaseURL, "/"),
		url.QueryEscape(strings.TrimSpace(id)),
		url.QueryEscape(idType))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("ruv: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ruv: calling the registry: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("ruv: registry returned %s", resp.Status)
	}

	var body struct {
		Data   []scoringRecord `json:"data"`
		Status string          `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("ruv: decoding response: %w", err)
	}
	if len(body.Data) == 0 {
		return nil, nil
	}

	rancher := &port.RUVRancher{NationalID: strings.TrimSpace(id)}
	for _, rec := range body.Data {
		if rancher.Name == "" {
			rancher.Name = strings.TrimSpace(rec.Ganadero)
			rancher.Phone = strings.TrimSpace(rec.Celular)
			rancher.Email = strings.TrimSpace(rec.Correo)
		}
		rancher.Holdings = append(rancher.Holdings, port.RUVHolding{
			Code:         rec.CodigoRUV,
			Name:         rec.NombrePredio,
			Department:   strings.TrimSpace(rec.Departamento),
			Municipality: strings.TrimSpace(rec.Municipio),
			Village:      strings.TrimSpace(rec.Vereda),
			Cycle:        atoi(rec.Ciclo),
			Year:         atoi(rec.Ano),
			VaccinatedAt: rec.FechaVacunacion,
			Herd:         rec.herd(),
		})
	}
	return rancher, nil
}
