// Command riskcheck runs the AI risk analysis once, on a synthetic applicant,
// through the same adapter the server uses. It is a smoke test for a provider
// or model change: it shows whether the model answers, how long it takes and
// what it returns, without touching any client data.
//
// Usage:
//
//	VERTEX_PROJECT=fedegan-prestia-30x \
//	GOOGLE_ACCESS_TOKEN="$(gcloud auth print-access-token)" \
//	go run ./cmd/riskcheck -prompt prompt.txt
//
// Without GOOGLE_ACCESS_TOKEN it uses Application Default Credentials.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.google.com/go/auth"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/port"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/ai"
)

// syntheticApplicant has the shape the risk service sends, with made-up data.
const syntheticApplicant = `{
  "client": {"name": "Ganadero de Prueba", "age": 47, "city": "Montería", "province": "Córdoba",
             "country": "Colombia", "isPEP": false, "blocked": false, "comments": ""},
  "riskData": {"monthlyIncome": "4200000", "monthlyExpenses": "1900000", "dependents": 2,
               "employmentType": "self_employed", "yearsInActivity": 14,
               "herd": {"vaccinatedHeadLatestCycle": 46, "notVaccinated": 0, "typology": "Doble Propósito"}},
  "history": {"previousLoans": 0, "latePayments": 0},
  "affordability": {"recognizedIncome": "4200000.00", "disposableIncome": "2300000.00",
                    "maxMonthlyPayment": "690000.00", "maxLimit": "3500000.00"},
  "currency": "COP"
}`

const defaultPrompt = `You assess the credit risk of a Colombian cattle rancher applying for a
short-term input loan. Weigh affordability first, then the herd that backs the
loan and its vaccination record, then any credit history. Be conservative when
data is missing.`

// staticToken hands the analyzer an access token obtained outside the program.
type staticToken struct{ value string }

func (t staticToken) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: t.value, Type: "Bearer", Expiry: time.Now().Add(30 * time.Minute)}, nil
}

func main() {
	promptFile := flag.String("prompt", "", "file with the operator prompt (default: a short built-in one)")
	model := flag.String("model", os.Getenv("RISK_ANALYSIS_MODEL"), "model to call (default: the adapter's)")
	effort := flag.String("effort", "high", "low, medium or high")
	flag.Parse()

	prompt := defaultPrompt
	if *promptFile != "" {
		b, err := os.ReadFile(*promptFile)
		if err != nil {
			log.Fatal(err)
		}
		prompt = string(b)
	}

	cfg := ai.GeminiConfig{
		Project:  os.Getenv("VERTEX_PROJECT"),
		Location: os.Getenv("VERTEX_LOCATION"),
		Model:    *model,
		Effort:   *effort,
	}
	if tok := os.Getenv("GOOGLE_ACCESS_TOKEN"); tok != "" {
		cfg.Credentials = auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: staticToken{tok}})
	}

	ctx := context.Background()
	analyzer, err := ai.NewGeminiAnalyzer(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	if analyzer == nil {
		log.Fatal("VERTEX_PROJECT is not set")
	}

	start := time.Now()
	result, err := analyzer.Analyze(ctx, port.RiskAnalysisRequest{Prompt: prompt, Applicant: syntheticApplicant})
	if err != nil {
		log.Fatalf("%s failed after %s: %v", analyzer.Name(), time.Since(start).Round(time.Millisecond), err)
	}
	fmt.Printf("%s answered in %s\n", analyzer.Name(), time.Since(start).Round(time.Millisecond))
	result.Raw = ""
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
}
