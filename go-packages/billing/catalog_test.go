package billing_test

import (
	"errors"
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/billing"
)

func TestNewCatalog_Validation(t *testing.T) {
	ok := billing.Price{ProviderPriceID: "p1", Kind: billing.KindPlan, ID: "pro"}
	tests := []struct {
		name    string
		cfg     billing.CatalogConfig
		wantErr bool
	}{
		{"valid", billing.CatalogConfig{FreePlanID: "free", Prices: []billing.Price{ok}}, false},
		{"no prices is allowed", billing.CatalogConfig{FreePlanID: "free"}, false},
		{"missing free plan", billing.CatalogConfig{Prices: []billing.Price{ok}}, true},
		{"empty provider price", billing.CatalogConfig{FreePlanID: "free", Prices: []billing.Price{{Kind: billing.KindPlan, ID: "x"}}}, true},
		{"empty target id", billing.CatalogConfig{FreePlanID: "free", Prices: []billing.Price{{ProviderPriceID: "p", Kind: billing.KindPlan}}}, true},
		{"unknown kind", billing.CatalogConfig{FreePlanID: "free", Prices: []billing.Price{{ProviderPriceID: "p", Kind: "bundle", ID: "x"}}}, true},
		{"duplicate provider price", billing.CatalogConfig{FreePlanID: "free", Prices: []billing.Price{ok, ok}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := billing.NewCatalog(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, billing.ErrInvalidConfig) {
				t.Errorf("err = %v, want ErrInvalidConfig", err)
			}
			if err == nil && c.FreePlanID() != "free" {
				t.Error("free plan lost")
			}
		})
	}
}
