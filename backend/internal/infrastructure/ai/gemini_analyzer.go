package ai

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/auth"
	"google.golang.org/genai"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// DefaultGeminiModel is the Gemini model used when none is configured.
const DefaultGeminiModel = "gemini-4-argon"

// DefaultVertexLocation is the Vertex AI location used when none is
// configured. The global endpoint serves the newest Gemini models and routes
// each request to available capacity.
const DefaultVertexLocation = "global"

// GeminiAnalyzer runs the operator-managed analysis prompt through Gemini on
// Vertex AI and returns a structured judgement.
//
// It authenticates as the runtime's service account through Application
// Default Credentials, so it needs no API key and its usage is billed to the
// Google Cloud project like any other service.
type GeminiAnalyzer struct {
	client    *genai.Client
	model     string
	thinking  genai.ThinkingLevel
	maxTokens int32
}

// GeminiConfig configures the Gemini-backed analyzer.
type GeminiConfig struct {
	// Project is the Google Cloud project Vertex AI is called and billed in.
	Project   string
	Location  string
	Model     string
	Effort    string
	MaxTokens int
	// Credentials replaces Application Default Credentials, for tools run
	// outside Google Cloud with a user's token.
	Credentials *auth.Credentials
	// BaseURL points the client at another endpoint; tests use it.
	BaseURL string
}

// NewGeminiAnalyzer builds an analyzer. It returns nil when no project is
// configured so the caller can fall back to the deterministic scorecard.
func NewGeminiAnalyzer(ctx context.Context, cfg GeminiConfig) (*GeminiAnalyzer, error) {
	if strings.TrimSpace(cfg.Project) == "" {
		return nil, nil
	}
	location := cfg.Location
	if location == "" {
		location = DefaultVertexLocation
	}
	model := cfg.Model
	if model == "" {
		model = DefaultGeminiModel
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16000
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:     genai.BackendVertexAI,
		Project:     cfg.Project,
		Location:    location,
		Credentials: cfg.Credentials,
		HTTPOptions: genai.HTTPOptions{BaseURL: cfg.BaseURL},
	})
	if err != nil {
		return nil, fmt.Errorf("creating the Vertex AI client: %w", err)
	}
	return &GeminiAnalyzer{
		client:    client,
		model:     model,
		thinking:  parseThinkingLevel(cfg.Effort),
		maxTokens: int32(maxTokens),
	}, nil
}

// parseThinkingLevel maps the provider-neutral effort setting onto Gemini's
// thinking levels. Anything above high stays at high, the highest level every
// Gemini model with thinking levels accepts.
func parseThinkingLevel(v string) genai.ThinkingLevel {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "low":
		return genai.ThinkingLevelLow
	case "medium":
		return genai.ThinkingLevelMedium
	default:
		return genai.ThinkingLevelHigh
	}
}

// Name identifies the analyzer in audit records and assessments.
func (a *GeminiAnalyzer) Name() string {
	return "gemini:" + a.model
}

// Analyze sends one applicant through the analysis prompt.
func (a *GeminiAnalyzer) Analyze(ctx context.Context, req port.RiskAnalysisRequest) (*port.RiskAnalysisResult, error) {
	resp, err := a.client.Models.GenerateContent(ctx, a.model,
		genai.Text(req.Applicant),
		&genai.GenerateContentConfig{
			// The operator prompt is the same for every applicant; the
			// applicant data is the only thing that varies per request.
			SystemInstruction:  genai.NewContentFromText(req.Prompt+outputContract, genai.RoleUser),
			ResponseMIMEType:   "application/json",
			ResponseJsonSchema: analysisSchema(),
			MaxOutputTokens:    a.maxTokens,
			ThinkingConfig:     &genai.ThinkingConfig{ThinkingLevel: a.thinking},
		})
	if err != nil {
		return nil, fmt.Errorf("risk analysis request failed: %w", err)
	}

	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return nil, fmt.Errorf("risk analysis declined by the model: %s", resp.PromptFeedback.BlockReason)
	}
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("risk analysis returned no candidates")
	}
	switch resp.Candidates[0].FinishReason {
	case genai.FinishReasonMaxTokens:
		return nil, fmt.Errorf("risk analysis truncated before completing: raise the token limit")
	case genai.FinishReasonSafety:
		return nil, fmt.Errorf("risk analysis declined by the model's safety filters")
	}

	payload := strings.TrimSpace(resp.Text())
	if payload == "" {
		return nil, fmt.Errorf("risk analysis returned no content")
	}
	return decodeAnalysis(payload)
}
