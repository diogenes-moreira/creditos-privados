package ruv

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/diogenes-moreira/creditos/backend/internal/domain/model"
)

// TestLiveProbe exercises the real registry end to end: it queries one national
// ID, maps the response onto the domain herd and scores it. Opt-in, because it
// needs credentials and network: set RUV_PROBE to a cédula to run it.
func TestLiveProbe(t *testing.T) {
	if os.Getenv("RUV_PROBE") == "" {
		t.Skip("set RUV_PROBE to run against the live registry")
	}
	c := New(Config{
		BaseURL:      os.Getenv("RUV_BASE_URL"),
		ClientID:     os.Getenv("RUV_CLIENT_ID"),
		ClientSecret: os.Getenv("RUV_CLIENT_SECRET"),
		Username:     os.Getenv("RUV_USERNAME"),
		Password:     os.Getenv("RUV_PASSWORD"),
	})
	if c == nil {
		t.Fatal("client not configured")
	}
	r, err := c.FindByNationalID(context.Background(), IDTypeCedula, os.Getenv("RUV_PROBE"))
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if r == nil {
		fmt.Println("sin registros para esa cédula")
		return
	}
	fmt.Printf("ganadero: %s | predios: %d | depto: %s | ciclos reportados: %d\n",
		r.Name, len(r.Holdings), r.Department(), r.ReportedCycles())
	for _, h := range r.Holdings {
		c := h.Herd.Classify()
		fmt.Printf("  predio %s (%s, %s) ciclo %d/%d -> %d cabezas, %d sin vacunar\n",
			h.Name, h.Municipality, h.Department, h.Cycle, h.Year, c.Total, c.NotVaccinated)
	}
	herd := r.AggregatedHerd()
	cls := herd.Classify()
	a := model.LivestockApplicant{
		NationalID: r.NationalID, Department: r.Department(), Herd: herd,
		Tenure: model.TenureOwned, HoldingStatus: model.HoldingActive,
		SeniorityCycles: r.ReportedCycles(), CompletedCycles: r.ReportedCycles(), ReportedCycles: r.ReportedCycles(),
	}
	res := model.AssessQuota(a, model.DefaultQuotaParams(), model.DefaultLivestockYields())
	fmt.Printf("\nTOTAL cabezas=%d  tipología=%s  UGG=%s  score=%d\n",
		cls.Total, res.Typology, res.UGG.StringFixed(1), res.Score.Score)
	fmt.Printf("CUPO = %s COP   techo activo: %s\n",
		res.Quota.Quota.StringFixed(0), res.Quota.ActiveCeiling)
}
