package envconfig

import (
	"os"
	"testing"

	"github.com/luxfi/broker/pkg/provider"
)

func TestRegisterFromEnv_NoVars(t *testing.T) {
	// Clear all provider env vars to ensure clean slate.
	envKeys := []string{
		"ALPACA_API_KEY", "IBKR_ACCESS_TOKEN", "BITGO_ACCESS_TOKEN",
		"FALCON_API_KEY", "FINIX_USERNAME", "SFOX_API_KEY",
		"COINBASE_API_KEY", "BINANCE_API_KEY", "KRAKEN_API_KEY",
		"GEMINI_API_KEY", "FIREBLOCKS_API_KEY", "CIRCLE_API_KEY",
		"TRADIER_ACCESS_TOKEN", "POLYGON_API_KEY", "CURRENCYCLOUD_LOGIN_ID",
		"LMAX_API_KEY",
	}
	for _, k := range envKeys {
		os.Unsetenv(k)
	}

	registry := provider.NewRegistry()
	n := RegisterFromEnv(registry)

	if n != 0 {
		t.Fatalf("expected 0 providers, got %d", n)
	}
	if len(registry.List()) != 0 {
		t.Fatalf("expected empty registry, got %v", registry.List())
	}
}

func TestRegisterFromEnv_SetsCount(t *testing.T) {
	// Clear everything first.
	envKeys := []string{
		"ALPACA_API_KEY", "IBKR_ACCESS_TOKEN", "BITGO_ACCESS_TOKEN",
		"FALCON_API_KEY", "FINIX_USERNAME", "SFOX_API_KEY",
		"COINBASE_API_KEY", "BINANCE_API_KEY", "KRAKEN_API_KEY",
		"GEMINI_API_KEY", "FIREBLOCKS_API_KEY", "CIRCLE_API_KEY",
		"TRADIER_ACCESS_TOKEN", "POLYGON_API_KEY", "CURRENCYCLOUD_LOGIN_ID",
		"LMAX_API_KEY",
	}
	for _, k := range envKeys {
		os.Unsetenv(k)
	}

	// Set 3 providers.
	t.Setenv("ALPACA_API_KEY", "test-key")
	t.Setenv("ALPACA_API_SECRET", "test-secret")
	t.Setenv("COINBASE_API_KEY", "test-cb-key")
	t.Setenv("CIRCLE_API_KEY", "test-circle-key")

	registry := provider.NewRegistry()
	n := RegisterFromEnv(registry)

	if n != 3 {
		t.Fatalf("expected 3 providers, got %d", n)
	}
	if len(registry.List()) != 3 {
		t.Fatalf("expected 3 in registry, got %d: %v", len(registry.List()), registry.List())
	}

	// Verify specific providers are registered.
	for _, name := range []string{"alpaca", "coinbase", "circle"} {
		if _, err := registry.Get(name); err != nil {
			t.Errorf("expected provider %q to be registered: %v", name, err)
		}
	}
}

func TestRegisterFromEnv_AllProviders(t *testing.T) {
	// Set all 16 provider keys.
	t.Setenv("ALPACA_API_KEY", "k")
	t.Setenv("IBKR_ACCESS_TOKEN", "k")
	t.Setenv("BITGO_ACCESS_TOKEN", "k")
	t.Setenv("FALCON_API_KEY", "k")
	t.Setenv("FINIX_USERNAME", "k")
	t.Setenv("SFOX_API_KEY", "k")
	t.Setenv("COINBASE_API_KEY", "k")
	t.Setenv("BINANCE_API_KEY", "k")
	t.Setenv("KRAKEN_API_KEY", "k")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("FIREBLOCKS_API_KEY", "k")
	t.Setenv("CIRCLE_API_KEY", "k")
	t.Setenv("TRADIER_ACCESS_TOKEN", "k")
	t.Setenv("POLYGON_API_KEY", "k")
	t.Setenv("CURRENCYCLOUD_LOGIN_ID", "k")
	t.Setenv("LMAX_API_KEY", "k")

	registry := provider.NewRegistry()
	n := RegisterFromEnv(registry)

	if n != 16 {
		t.Fatalf("expected 16 providers, got %d", n)
	}
}
