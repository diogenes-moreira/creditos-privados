// Command ruvtiers computes the credit tier of every rancher in a list.
//
// It reads a CSV with a "row" column and an "identificacion" column, queries
// the RUV for each national ID and writes a CSV with one result per input row:
// the pilot's credit tier and, for comparison, the quota of the four-ceiling
// engine. Both come from the server's own LivestockQuotaService, so a list
// scored here matches what the quota lookup shows, except that the engine
// runs on the shipped default parameters rather than the policy active in
// the database.
//
// Everyone in the list is taken to be a buyer of the campaign the list comes
// from. The credit bureaus and land ownership cannot be read from the
// registry, so they are reported as pending rather than assumed.
//
// Usage:
//
//	RUV_BASE_URL=... RUV_CLIENT_ID=... RUV_CLIENT_SECRET=... \
//	RUV_USERNAME=... RUV_PASSWORD=... \
//	go run ./cmd/ruvtiers -in ids.csv -out tiers.csv
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/diogenes-moreira/creditos/backend/internal/application/service"
	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
	"github.com/diogenes-moreira/creditos/backend/internal/infrastructure/ruv"
	"github.com/shopspring/decimal"
)

// normalizeID turns the list's spellings of a cédula into the bare number the
// registry is keyed on: "77.027.228" and "77017779-3" (a cédula written with
// its NIT check digit) become "77027228" and "77017779".
func normalizeID(raw string) string {
	id := strings.TrimSpace(raw)
	if i := strings.Index(id, "-"); i >= 0 {
		id = id[:i]
	}
	var b strings.Builder
	for _, r := range id {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// shortfall names what kept the rancher from the next layer up: the unmet or
// unverified criteria of the lowest layer not reached.
func shortfall(a model.TierAssessment) string {
	for _, t := range a.Tiers {
		if t.Qualifies {
			continue
		}
		var missing []string
		for _, c := range t.Criteria {
			switch c.Status {
			case model.CheckUnmet:
				missing = append(missing, fmt.Sprintf("%s %s (tiene %s)", c.Code, c.Required, c.Actual))
			case model.CheckUnverified:
				missing = append(missing, c.Code+" sin verificar")
			}
		}
		return t.Code + ": " + strings.Join(missing, "; ")
	}
	return ""
}

var hundred = decimal.NewFromInt(100)

// discard satisfies the assessment repository: a batch run is a study, not a
// lookup anyone made, so nothing is stored.
type discard struct{}

func (discard) Create(context.Context, *model.LivestockQuotaAssessment) error { return nil }

func main() {
	in := flag.String("in", "", "input CSV with row,identificacion")
	out := flag.String("out", "", "output CSV")
	pause := flag.Duration("pause", 250*time.Millisecond, "wait between registry queries")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("-in and -out are required")
	}

	client := ruv.New(ruv.Config{
		BaseURL:      os.Getenv("RUV_BASE_URL"),
		ClientID:     os.Getenv("RUV_CLIENT_ID"),
		ClientSecret: os.Getenv("RUV_CLIENT_SECRET"),
		Username:     os.Getenv("RUV_USERNAME"),
		Password:     os.Getenv("RUV_PASSWORD"),
	})
	if client == nil {
		log.Fatal("RUV credentials are not set")
	}

	f, err := os.Open(*in)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		log.Fatal(err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	rowCol, okRow := col["row"]
	idCol, okID := col["identificacion"]
	if !okRow || !okID {
		log.Fatal(`the input needs "row" and "identificacion" columns`)
	}

	of, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer of.Close()
	w := csv.NewWriter(of)
	defer w.Flush()
	_ = w.Write([]string{"row", "cedula", "estado", "disponible", "capa", "disponible_provisional", "capa_provisional",
		"cabezas_ultimo_ciclo", "no_vacunadas_ultimo_ciclo", "ultimo_ciclo", "ciclos_reportados",
		"ciclos_completos", "pct_vacunacion", "pct_ciclos_completos", "predio_activo",
		"pendiente", "falta_para_siguiente",
		"cupo_modelo", "techo_activo", "score", "grado", "tipologia"})

	tiers := model.DefaultCreditTiers()
	buyer := true
	now := time.Now()
	ctx := context.Background()
	quotas := service.NewLivestockQuotaService(discard{}, "COP").WithRegistry(client).
		WithClock(func() time.Time { return now })

	counts := map[string]int{}
	n := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		n++
		row, id := rec[rowCol], normalizeID(rec[idCol])
		if id == "" {
			_ = w.Write([]string{row, "", "sin_cedula"})
			counts["sin_cedula"]++
			continue
		}

		result, err := quotas.AssessFromRegistry(ctx, ruv.IDTypeCedula, id, "ruvtiers", "")
		if errors.Is(err, service.ErrNotInRegistry) {
			_ = w.Write([]string{row, id, "no_en_ruv", "0", "", "0"})
			counts["no_en_ruv"]++
			time.Sleep(*pause)
			continue
		}
		if err != nil {
			log.Printf("row %s: %v", row, err)
			_ = w.Write([]string{row, id, "error_ruv"})
			counts["error_ruv"]++
			time.Sleep(*pause)
			continue
		}
		rancher := result.Rancher

		vr := rancher.VaccinationRecord(now)
		a := model.AssessCreditTier(model.TierEvidence{
			IsCedula:      true,
			CampaignBuyer: &buyer,
			Record:        vr,
		}, tiers)

		status := "sin_capa"
		switch {
		case !a.Eligible:
			status = "no_elegible"
		case a.Code != "":
			status = "preaprobado"
		case a.ProvisionalCode != "":
			status = "provisional"
		}
		counts[status]++
		counts["capa_provisional_"+a.ProvisionalCode]++

		active := "no"
		if vr.HoldingActive {
			active = "si"
		}
		_ = w.Write([]string{row, id, status, a.Amount.StringFixed(0), a.Code, a.ProvisionalAmount.StringFixed(0), a.ProvisionalCode,
			fmt.Sprint(vr.VaccinatedHeads), fmt.Sprint(vr.NotVaccinated),
			fmt.Sprintf("%d-%d", vr.LatestYear, vr.LatestCycle),
			fmt.Sprint(vr.ReportedCycles), fmt.Sprint(vr.CompleteCycles),
			vr.VaccinationRate().Mul(hundred).Round(1).String(),
			vr.CompleteCycleRatio().Mul(hundred).Round(1).String(),
			active, strings.Join(a.Pending, ", "), shortfall(a),
			result.Record.Quota.StringFixed(0), result.Assessment.Quota.ActiveCeilingCode,
			fmt.Sprint(result.Assessment.Score.Score), result.Assessment.Score.Grade(),
			string(result.Assessment.Typology)})
		w.Flush()
		time.Sleep(*pause)
	}
	log.Printf("%d rows: %v", n, counts)
}
