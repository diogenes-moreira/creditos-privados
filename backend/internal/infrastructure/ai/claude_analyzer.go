// Package ai adapts external language-model providers to the domain's risk
// analyzer port.
package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
)

// DefaultModel is the Claude model used when none is configured.
const DefaultModel = "claude-opus-5"

// ClaudeAnalyzer runs the operator-managed analysis prompt through the Claude
// API and returns a structured judgement.
type ClaudeAnalyzer struct {
	client    anthropic.Client
	model     string
	effort    anthropic.OutputConfigEffort
	maxTokens int64
}

// Config configures the Claude-backed analyzer.
type Config struct {
	APIKey    string
	Model     string
	Effort    string
	MaxTokens int
}

// NewClaudeAnalyzer builds an analyzer. It returns nil when no API key is
// configured so the caller can fall back to the deterministic scorecard.
func NewClaudeAnalyzer(cfg Config) *ClaudeAnalyzer {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil
	}
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	maxTokens := int64(cfg.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 16000
	}
	return &ClaudeAnalyzer{
		client:    anthropic.NewClient(option.WithAPIKey(cfg.APIKey)),
		model:     model,
		effort:    parseEffort(cfg.Effort),
		maxTokens: maxTokens,
	}
}

func parseEffort(v string) anthropic.OutputConfigEffort {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "low":
		return anthropic.OutputConfigEffortLow
	case "medium":
		return anthropic.OutputConfigEffortMedium
	case "xhigh":
		return anthropic.OutputConfigEffortXhigh
	case "max":
		return anthropic.OutputConfigEffortMax
	default:
		return anthropic.OutputConfigEffortHigh
	}
}

// Name identifies the analyzer in audit records and assessments.
func (a *ClaudeAnalyzer) Name() string {
	return "claude:" + a.model
}

// Analyze sends one applicant through the analysis prompt.
func (a *ClaudeAnalyzer) Analyze(ctx context.Context, req port.RiskAnalysisRequest) (*port.RiskAnalysisResult, error) {
	// The operator prompt is stable across applicants, so it carries the cache
	// breakpoint; the applicant data varies and goes in the user turn after it.
	system := []anthropic.TextBlockParam{{
		Text:         req.Prompt + outputContract,
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
	}}

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: a.maxTokens,
		System:    system,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: a.effort,
			Format: anthropic.JSONOutputFormatParam{Schema: analysisSchema()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.Applicant)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("risk analysis request failed: %w", err)
	}

	if resp.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("risk analysis declined by the model: %s", resp.StopDetails.Explanation)
	}
	if resp.StopReason == anthropic.StopReasonMaxTokens {
		return nil, fmt.Errorf("risk analysis truncated before completing: raise the token limit")
	}

	var raw strings.Builder
	for _, block := range resp.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			raw.WriteString(text.Text)
		}
	}
	payload := strings.TrimSpace(raw.String())
	if payload == "" {
		return nil, fmt.Errorf("risk analysis returned no content")
	}

	return decodeAnalysis(payload)
}
