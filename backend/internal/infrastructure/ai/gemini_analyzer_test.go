package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// staticToken stands in for Application Default Credentials in tests.
type staticToken struct{}

func (staticToken) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: "test-token", Type: "Bearer", Expiry: time.Now().Add(time.Hour)}, nil
}

const validAnswer = `{
  "score": 72, "grade": " b ", "decision": "Approve",
  "probabilityOfDefault": "0.08", "recommendedLimit": "1500000",
  "recommendedInstallments": 6, "recommendedInterestRate": "26",
  "maxMonthlyPayment": "280000",
  "summary": "Stable herd and regular vaccination.",
  "factors": [{"code": "herd", "label": "Herd size", "value": "45", "impact": "Positive", "comment": "Enough cattle to back the limit."}]
}`

// fakeVertex answers generateContent with body and records the last request.
func fakeVertex(t *testing.T, body string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var last map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &last)
		assert.Contains(t, r.URL.Path, "/projects/test-project/locations/global/publishers/google/models/gemini-test:generateContent")
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

func newTestAnalyzer(t *testing.T, baseURL, effort string) *GeminiAnalyzer {
	t.Helper()
	a, err := NewGeminiAnalyzer(context.Background(), GeminiConfig{
		Project:     "test-project",
		Model:       "gemini-test",
		Effort:      effort,
		Credentials: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: staticToken{}}),
		BaseURL:     baseURL,
	})
	require.NoError(t, err)
	require.NotNil(t, a)
	return a
}

func candidate(text, finish string) string {
	b, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{
		"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": text}}},
		"finishReason": finish,
	}}})
	return string(b)
}

func TestGeminiAnalyzerWithoutAProjectIsDisabled(t *testing.T) {
	a, err := NewGeminiAnalyzer(context.Background(), GeminiConfig{})
	require.NoError(t, err)
	assert.Nil(t, a, "no project means the scorecard handles the analysis")
}

func TestGeminiAnalyzerReturnsTheNormalisedJudgement(t *testing.T) {
	srv, _ := fakeVertex(t, candidate(validAnswer, "STOP"))
	a := newTestAnalyzer(t, srv.URL, "high")

	got, err := a.Analyze(context.Background(), port.RiskAnalysisRequest{Prompt: "Judge the rancher.", Applicant: "herd: 45"})

	require.NoError(t, err)
	assert.Equal(t, 72, got.Score)
	assert.Equal(t, "B", got.Grade, "grades are upper-cased and trimmed")
	assert.Equal(t, "approve", got.Decision, "decisions are lower-cased")
	assert.Equal(t, "1500000", got.RecommendedLimit)
	assert.Equal(t, 6, got.RecommendedInstallments)
	require.Len(t, got.Factors, 1)
	assert.Equal(t, "positive", got.Factors[0].Impact)
	assert.Equal(t, "gemini:gemini-test", a.Name())
}

func TestGeminiAnalyzerSendsThePromptTheSchemaAndTheThinkingLevel(t *testing.T) {
	srv, last := fakeVertex(t, candidate(validAnswer, "STOP"))
	a := newTestAnalyzer(t, srv.URL, "low")

	_, err := a.Analyze(context.Background(), port.RiskAnalysisRequest{Prompt: "Judge the rancher.", Applicant: "herd: 45"})
	require.NoError(t, err)

	req := *last
	raw, _ := json.Marshal(req)
	body := string(raw)
	assert.Contains(t, body, "Judge the rancher.", "the operator prompt is the system instruction")
	assert.Contains(t, body, "Return your analysis as JSON", "the output contract follows it")
	assert.Contains(t, body, "herd: 45", "the applicant is the user turn")
	gen := req["generationConfig"].(map[string]any)
	assert.Equal(t, "application/json", gen["responseMimeType"])
	assert.NotNil(t, gen["responseJsonSchema"], "the same schema as every provider")
	assert.Equal(t, "LOW", gen["thinkingConfig"].(map[string]any)["thinkingLevel"])
}

func TestGeminiAnalyzerReportsATruncatedAnswer(t *testing.T) {
	srv, _ := fakeVertex(t, candidate(`{"score": 7`, "MAX_TOKENS"))
	_, err := newTestAnalyzer(t, srv.URL, "").Analyze(context.Background(), port.RiskAnalysisRequest{Applicant: "x"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncated")
}

func TestGeminiAnalyzerReportsABlockedPrompt(t *testing.T) {
	srv, _ := fakeVertex(t, `{"promptFeedback": {"blockReason": "SAFETY"}}`)
	_, err := newTestAnalyzer(t, srv.URL, "").Analyze(context.Background(), port.RiskAnalysisRequest{Applicant: "x"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "declined")
}

func TestGeminiAnalyzerRejectsAnAnswerThatIsNotJSON(t *testing.T) {
	srv, _ := fakeVertex(t, candidate("I think this rancher is fine.", "STOP"))
	_, err := newTestAnalyzer(t, srv.URL, "").Analyze(context.Background(), port.RiskAnalysisRequest{Applicant: "x"})

	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "decode"), err.Error())
}

func TestParseThinkingLevelKeepsHighAsTheCeiling(t *testing.T) {
	for in, want := range map[string]string{"low": "LOW", "medium": "MEDIUM", "high": "HIGH", "xhigh": "HIGH", "max": "HIGH", "": "HIGH"} {
		assert.Equal(t, want, string(parseThinkingLevel(in)), in)
	}
}
