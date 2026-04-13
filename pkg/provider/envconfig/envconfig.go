// Package envconfig registers broker providers from environment variables.
// It imports all 16 provider sub-packages so callers don't have to.
package envconfig

import (
	"log/slog"
	"os"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/provider/apex"
	"github.com/luxfi/broker/pkg/provider/alpaca"
	"github.com/luxfi/broker/pkg/provider/alpaca_omnisub"
	"github.com/luxfi/broker/pkg/provider/binance"
	"github.com/luxfi/broker/pkg/provider/bitgo"
	"github.com/luxfi/broker/pkg/provider/circle"
	"github.com/luxfi/broker/pkg/provider/coinbase"
	"github.com/luxfi/broker/pkg/provider/currencycloud"
	"github.com/luxfi/broker/pkg/provider/falcon"
	"github.com/luxfi/broker/pkg/provider/finix"
	"github.com/luxfi/broker/pkg/provider/fireblocks"
	"github.com/luxfi/broker/pkg/provider/gemini"
	"github.com/luxfi/broker/pkg/provider/ibkr"
	"github.com/luxfi/broker/pkg/provider/kraken"
	"github.com/luxfi/broker/pkg/provider/lmax"
	"github.com/luxfi/broker/pkg/provider/lx"
	"github.com/luxfi/broker/pkg/provider/polygon"
	"github.com/luxfi/broker/pkg/provider/sfox"
	"github.com/luxfi/broker/pkg/provider/tradier"

	// International adapters
	"github.com/luxfi/broker/pkg/provider/btgpactual"
	"github.com/luxfi/broker/pkg/provider/ig"
	"github.com/luxfi/broker/pkg/provider/questrade"
	"github.com/luxfi/broker/pkg/provider/saxo"
	"github.com/luxfi/broker/pkg/provider/stake"
	"github.com/luxfi/broker/pkg/provider/zerodha"
)

// RegisterFromEnv reads provider environment variables and registers
// all configured providers on the given registry. Returns the count
// of providers registered. This is the standard way to configure
// broker providers -- any ATS, BD, or TA can call this.
func RegisterFromEnv(registry *provider.Registry) int {
	n := 0

	// Lux DEX (luxfi/dex precompile, ZAP transport) — first-class venue
	// for any chain shipping the DEX precompile (Lux mainnet, Lux subnets,
	// Liquidity L1). Auto-registers when LX_DEX_ADDR is set or
	// ENVIRONMENT=local.
	if addr := os.Getenv("LX_DEX_ADDR"); addr != "" || os.Getenv("ENVIRONMENT") == "local" {
		registry.Register(lx.New(lx.Config{
			DEXAddr:     envOr("LX_DEX_ADDR", lx.DefaultDEXAddr),
			MPCAddr:     envOr("LX_MPC_ADDR", lx.DefaultMPCAddr),
			USDLAddress: os.Getenv("LX_USDL_ADDR"),
		}))
		slog.Info("provider registered", "name", "lx", "transport", "zap")
		n++
	}

	if key := os.Getenv("ALPACA_API_KEY"); key != "" {
		registry.Register(alpaca.New(alpaca.Config{
			BaseURL:   envOr("ALPACA_BASE_URL", alpaca.SandboxURL),
			APIKey:    key,
			APISecret: os.Getenv("ALPACA_API_SECRET"),
		}))
		slog.Info("provider registered", "name", "alpaca")
		n++
	}

	if key := os.Getenv("ALPACA_OMNISUB_API_KEY"); key != "" {
		registry.Register(alpaca_omnisub.New(alpaca_omnisub.Config{
			BaseURL:          envOr("ALPACA_OMNISUB_BASE_URL", alpaca_omnisub.SandboxURL),
			APIKey:           key,
			APISecret:        os.Getenv("ALPACA_OMNISUB_API_SECRET"),
			OmnibusAccountID: os.Getenv("ALPACA_OMNISUB_OMNIBUS_ACCOUNT_ID"),
		}))
		slog.Info("provider registered", "name", "alpaca_omnisub")
		n++
	}

	if token := os.Getenv("IBKR_ACCESS_TOKEN"); token != "" {
		registry.Register(ibkr.New(ibkr.Config{
			GatewayURL:  envOr("IBKR_GATEWAY_URL", ibkr.DefaultGatewayURL),
			AccountID:   os.Getenv("IBKR_ACCOUNT_ID"),
			AccessToken: token,
			ConsumerKey: os.Getenv("IBKR_CONSUMER_KEY"),
		}))
		slog.Info("provider registered", "name", "ibkr")
		n++
	}

	if token := os.Getenv("BITGO_ACCESS_TOKEN"); token != "" {
		registry.Register(bitgo.New(bitgo.Config{
			BaseURL:     envOr("BITGO_BASE_URL", bitgo.TestURL),
			AccessToken: token,
			Enterprise:  os.Getenv("BITGO_ENTERPRISE"),
		}))
		slog.Info("provider registered", "name", "bitgo")
		n++
	}

	if key := os.Getenv("FALCON_API_KEY"); key != "" {
		registry.Register(falcon.New(falcon.Config{
			BaseURL:    envOr("FALCON_BASE_URL", falcon.SandboxURL),
			APIKey:     key,
			APISecret:  os.Getenv("FALCON_API_SECRET"),
			Passphrase: os.Getenv("FALCON_PASSPHRASE"),
		}))
		slog.Info("provider registered", "name", "falcon")
		n++
	}

	if user := os.Getenv("FINIX_USERNAME"); user != "" {
		registry.Register(finix.New(finix.Config{
			BaseURL:  envOr("FINIX_BASE_URL", finix.SandboxURL),
			Username: user,
			Password: os.Getenv("FINIX_PASSWORD"),
		}))
		slog.Info("provider registered", "name", "finix")
		n++
	}

	if key := os.Getenv("SFOX_API_KEY"); key != "" {
		registry.Register(sfox.New(sfox.Config{
			BaseURL: envOr("SFOX_BASE_URL", sfox.ProdURL),
			APIKey:  key,
		}))
		slog.Info("provider registered", "name", "sfox")
		n++
	}

	if key := os.Getenv("COINBASE_API_KEY"); key != "" {
		registry.Register(coinbase.New(coinbase.Config{
			BaseURL:   envOr("COINBASE_BASE_URL", coinbase.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("COINBASE_API_SECRET"),
		}))
		slog.Info("provider registered", "name", "coinbase")
		n++
	}

	if key := os.Getenv("BINANCE_API_KEY"); key != "" {
		registry.Register(binance.New(binance.Config{
			BaseURL:   envOr("BINANCE_BASE_URL", binance.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("BINANCE_API_SECRET"),
		}))
		slog.Info("provider registered", "name", "binance")
		n++
	}

	if key := os.Getenv("KRAKEN_API_KEY"); key != "" {
		registry.Register(kraken.New(kraken.Config{
			BaseURL:   envOr("KRAKEN_BASE_URL", kraken.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("KRAKEN_API_SECRET"),
		}))
		slog.Info("provider registered", "name", "kraken")
		n++
	} else if os.Getenv("ENABLE_PUBLIC_DATA") != "" || os.Getenv("ENVIRONMENT") == "local" {
		// Kraken public API works without keys for market data (read-only).
		registry.Register(kraken.New(kraken.Config{
			BaseURL: envOr("KRAKEN_BASE_URL", kraken.ProdURL),
		}))
		slog.Info("provider registered", "name", "kraken", "mode", "public-data")
		n++
	}

	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		registry.Register(gemini.New(gemini.Config{
			BaseURL:   envOr("GEMINI_BASE_URL", gemini.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("GEMINI_API_SECRET"),
		}))
		slog.Info("provider registered", "name", "gemini")
		n++
	} else if os.Getenv("ENABLE_PUBLIC_DATA") != "" || os.Getenv("ENVIRONMENT") == "local" {
		// Gemini public API works without keys for market data (read-only).
		registry.Register(gemini.New(gemini.Config{
			BaseURL: envOr("GEMINI_BASE_URL", gemini.ProdURL),
		}))
		slog.Info("provider registered", "name", "gemini", "mode", "public-data")
		n++
	}

	if key := os.Getenv("FIREBLOCKS_API_KEY"); key != "" {
		registry.Register(fireblocks.New(fireblocks.Config{
			BaseURL:       envOr("FIREBLOCKS_BASE_URL", fireblocks.ProdURL),
			APIKey:        key,
			PrivateKeyPEM: os.Getenv("FIREBLOCKS_PRIVATE_KEY"),
		}))
		slog.Info("provider registered", "name", "fireblocks")
		n++
	}

	if key := os.Getenv("CIRCLE_API_KEY"); key != "" {
		registry.Register(circle.New(circle.Config{
			BaseURL: envOr("CIRCLE_BASE_URL", circle.SandboxURL),
			APIKey:  key,
		}))
		slog.Info("provider registered", "name", "circle")
		n++
	}

	if token := os.Getenv("TRADIER_ACCESS_TOKEN"); token != "" {
		registry.Register(tradier.New(tradier.Config{
			BaseURL:     envOr("TRADIER_BASE_URL", tradier.SandboxURL),
			AccessToken: token,
			AccountID:   os.Getenv("TRADIER_ACCOUNT_ID"),
		}))
		slog.Info("provider registered", "name", "tradier")
		n++
	}

	if key := os.Getenv("POLYGON_API_KEY"); key != "" {
		registry.Register(polygon.New(polygon.Config{
			BaseURL: envOr("POLYGON_BASE_URL", polygon.ProdURL),
			APIKey:  key,
		}))
		slog.Info("provider registered", "name", "polygon")
		n++
	}

	if login := os.Getenv("CURRENCYCLOUD_LOGIN_ID"); login != "" {
		registry.Register(currencycloud.New(currencycloud.Config{
			BaseURL: envOr("CURRENCYCLOUD_BASE_URL", currencycloud.DemoURL),
			LoginID: login,
			APIKey:  os.Getenv("CURRENCYCLOUD_API_KEY"),
		}))
		slog.Info("provider registered", "name", "currencycloud")
		n++
	}

	if key := os.Getenv("LMAX_API_KEY"); key != "" {
		registry.Register(lmax.New(lmax.Config{
			BaseURL:  envOr("LMAX_BASE_URL", lmax.SandboxURL),
			APIKey:   key,
			Username: os.Getenv("LMAX_USERNAME"),
			Password: os.Getenv("LMAX_PASSWORD"),
		}))
		slog.Info("provider registered", "name", "lmax")
		n++
	}

	if key := os.Getenv("APEX_API_KEY"); key != "" {
		sandbox := os.Getenv("APEX_SANDBOX") == "true" || os.Getenv("APEX_SANDBOX") == "1"
		registry.Register(apex.New(key, os.Getenv("APEX_API_SECRET"), sandbox))
		slog.Info("provider registered", "name", "apex")
		n++
	}

	// --- International adapters ---

	if id := os.Getenv("BTG_CLIENT_ID"); id != "" {
		registry.Register(btgpactual.New(btgpactual.Config{
			BaseURL:      envOr("BTG_BASE_URL", btgpactual.SandboxURL),
			ClientID:     id,
			ClientSecret: os.Getenv("BTG_CLIENT_SECRET"),
			AccountID:    os.Getenv("BTG_ACCOUNT_ID"),
		}))
		slog.Info("provider registered", "name", "btgpactual")
		n++
	}

	if key := os.Getenv("IG_API_KEY"); key != "" {
		registry.Register(ig.New(ig.Config{
			BaseURL:   envOr("IG_BASE_URL", ig.DemoURL),
			APIKey:    key,
			Username:  os.Getenv("IG_USERNAME"),
			Password:  os.Getenv("IG_PASSWORD"),
			AccountID: os.Getenv("IG_ACCOUNT_ID"),
		}))
		slog.Info("provider registered", "name", "ig")
		n++
	}

	if key := os.Getenv("ZERODHA_API_KEY"); key != "" {
		registry.Register(zerodha.New(zerodha.Config{
			BaseURL:   envOr("ZERODHA_BASE_URL", zerodha.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("ZERODHA_API_SECRET"),
		}))
		slog.Info("provider registered", "name", "zerodha")
		n++
	}

	if token := os.Getenv("SAXO_ACCESS_TOKEN"); token != "" {
		registry.Register(saxo.New(saxo.Config{
			BaseURL:      envOr("SAXO_BASE_URL", saxo.SimURL),
			AccessToken:  token,
			ClientID:     os.Getenv("SAXO_CLIENT_ID"),
			ClientSecret: os.Getenv("SAXO_CLIENT_SECRET"),
			AccountKey:   os.Getenv("SAXO_ACCOUNT_KEY"),
		}))
		slog.Info("provider registered", "name", "saxo")
		n++
	}

	if token := os.Getenv("QUESTRADE_REFRESH_TOKEN"); token != "" {
		registry.Register(questrade.New(questrade.Config{
			BaseURL:      envOr("QUESTRADE_BASE_URL", questrade.ProdURL),
			RefreshToken: token,
			AccountID:    os.Getenv("QUESTRADE_ACCOUNT_ID"),
		}))
		slog.Info("provider registered", "name", "questrade")
		n++
	}

	if key := os.Getenv("STAKE_API_KEY"); key != "" {
		registry.Register(stake.New(stake.Config{
			BaseURL:   envOr("STAKE_BASE_URL", stake.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("STAKE_API_SECRET"),
		}))
		slog.Info("provider registered", "name", "stake")
		n++
	}

	return n
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
