package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/commerce/payment/processor"
	btprovider "github.com/hanzoai/commerce/payment/providers/braintree"

	"github.com/luxfi/broker/pkg/api"
	"github.com/luxfi/broker/pkg/funding"
	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/provider/alpaca"
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
	"github.com/luxfi/broker/pkg/provider/polygon"
	"github.com/luxfi/broker/pkg/provider/sfox"
	"github.com/luxfi/broker/pkg/provider/tradier"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	listenAddr := envOr("BROKER_LISTEN", ":8090")

	registry := provider.NewRegistry()

	// Register Alpaca provider (equities + crypto)
	if key := os.Getenv("ALPACA_API_KEY"); key != "" {
		registry.Register(alpaca.New(alpaca.Config{
			BaseURL:   envOr("ALPACA_BASE_URL", alpaca.SandboxURL),
			APIKey:    key,
			APISecret: os.Getenv("ALPACA_API_SECRET"),
		}))
		log.Info().Msg("Alpaca provider registered")
	}

	// Register IBKR provider (equities, options, futures, forex)
	if token := os.Getenv("IBKR_ACCESS_TOKEN"); token != "" {
		registry.Register(ibkr.New(ibkr.Config{
			GatewayURL:  envOr("IBKR_GATEWAY_URL", ibkr.DefaultGatewayURL),
			AccountID:   os.Getenv("IBKR_ACCOUNT_ID"),
			AccessToken: token,
			ConsumerKey: os.Getenv("IBKR_CONSUMER_KEY"),
		}))
		log.Info().Msg("IBKR provider registered")
	}

	// Register BitGo provider (crypto custody + Prime trading)
	if token := os.Getenv("BITGO_ACCESS_TOKEN"); token != "" {
		registry.Register(bitgo.New(bitgo.Config{
			BaseURL:     envOr("BITGO_BASE_URL", bitgo.TestURL),
			AccessToken: token,
			Enterprise:  os.Getenv("BITGO_ENTERPRISE"),
		}))
		log.Info().Msg("BitGo provider registered")
	}

	// Register FalconX provider (institutional crypto RFQ)
	if key := os.Getenv("FALCON_API_KEY"); key != "" {
		registry.Register(falcon.New(falcon.Config{
			BaseURL:    envOr("FALCON_BASE_URL", falcon.SandboxURL),
			APIKey:     key,
			APISecret:  os.Getenv("FALCON_API_SECRET"),
			Passphrase: os.Getenv("FALCON_PASSPHRASE"),
		}))
		log.Info().Msg("FalconX provider registered")
	}

	// Register Finix provider (payment processing)
	if user := os.Getenv("FINIX_USERNAME"); user != "" {
		registry.Register(finix.New(finix.Config{
			BaseURL:  envOr("FINIX_BASE_URL", finix.SandboxURL),
			Username: user,
			Password: os.Getenv("FINIX_PASSWORD"),
		}))
		log.Info().Msg("Finix provider registered")
	}

	// Register SFOX provider (crypto prime dealer / smart routing)
	if key := os.Getenv("SFOX_API_KEY"); key != "" {
		registry.Register(sfox.New(sfox.Config{
			BaseURL: envOr("SFOX_BASE_URL", sfox.ProdURL),
			APIKey:  key,
		}))
		log.Info().Msg("SFOX provider registered")
	}

	// Register Coinbase provider (Advanced Trade API)
	if key := os.Getenv("COINBASE_API_KEY"); key != "" {
		registry.Register(coinbase.New(coinbase.Config{
			BaseURL:   envOr("COINBASE_BASE_URL", coinbase.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("COINBASE_API_SECRET"),
		}))
		log.Info().Msg("Coinbase provider registered")
	}

	// Register Binance provider (US or global)
	if key := os.Getenv("BINANCE_API_KEY"); key != "" {
		registry.Register(binance.New(binance.Config{
			BaseURL:   envOr("BINANCE_BASE_URL", binance.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("BINANCE_API_SECRET"),
		}))
		log.Info().Msg("Binance provider registered")
	}

	// Register Kraken provider
	if key := os.Getenv("KRAKEN_API_KEY"); key != "" {
		registry.Register(kraken.New(kraken.Config{
			BaseURL:   envOr("KRAKEN_BASE_URL", kraken.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("KRAKEN_API_SECRET"),
		}))
		log.Info().Msg("Kraken provider registered")
	}

	// Register Gemini provider
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		registry.Register(gemini.New(gemini.Config{
			BaseURL:   envOr("GEMINI_BASE_URL", gemini.ProdURL),
			APIKey:    key,
			APISecret: os.Getenv("GEMINI_API_SECRET"),
		}))
		log.Info().Msg("Gemini provider registered")
	}

	// Fireblocks — institutional crypto custody
	if key := os.Getenv("FIREBLOCKS_API_KEY"); key != "" {
		registry.Register(fireblocks.New(fireblocks.Config{
			BaseURL:       envOr("FIREBLOCKS_BASE_URL", fireblocks.ProdURL),
			APIKey:        key,
			PrivateKeyPEM: os.Getenv("FIREBLOCKS_PRIVATE_KEY"),
		}))
		log.Info().Msg("Fireblocks provider registered")
	}

	// Circle — USDC/stablecoin operations
	if key := os.Getenv("CIRCLE_API_KEY"); key != "" {
		registry.Register(circle.New(circle.Config{
			BaseURL: envOr("CIRCLE_BASE_URL", circle.SandboxURL),
			APIKey:  key,
		}))
		log.Info().Msg("Circle provider registered")
	}

	// Tradier — equities + options trading
	if token := os.Getenv("TRADIER_ACCESS_TOKEN"); token != "" {
		registry.Register(tradier.New(tradier.Config{
			BaseURL:     envOr("TRADIER_BASE_URL", tradier.SandboxURL),
			AccessToken: token,
			AccountID:   os.Getenv("TRADIER_ACCOUNT_ID"),
		}))
		log.Info().Msg("Tradier provider registered")
	}

	// Polygon.io — market data aggregator (stocks, FX, crypto)
	if key := os.Getenv("POLYGON_API_KEY"); key != "" {
		registry.Register(polygon.New(polygon.Config{
			BaseURL: envOr("POLYGON_BASE_URL", polygon.ProdURL),
			APIKey:  key,
		}))
		log.Info().Msg("Polygon provider registered")
	}

	// CurrencyCloud (Visa) — institutional FX + cross-border
	if login := os.Getenv("CURRENCYCLOUD_LOGIN_ID"); login != "" {
		registry.Register(currencycloud.New(currencycloud.Config{
			BaseURL: envOr("CURRENCYCLOUD_BASE_URL", currencycloud.DemoURL),
			LoginID: login,
			APIKey:  os.Getenv("CURRENCYCLOUD_API_KEY"),
		}))
		log.Info().Msg("CurrencyCloud provider registered")
	}

	// LMAX Digital — institutional FX/crypto exchange
	if key := os.Getenv("LMAX_API_KEY"); key != "" {
		registry.Register(lmax.New(lmax.Config{
			BaseURL:  envOr("LMAX_BASE_URL", lmax.SandboxURL),
			APIKey:   key,
			Username: os.Getenv("LMAX_USERNAME"),
			Password: os.Getenv("LMAX_PASSWORD"),
		}))
		log.Info().Msg("LMAX provider registered")
	}

	if len(registry.List()) == 0 {
		log.Fatal().Msg("No providers configured. Set provider env vars (ALPACA_API_KEY, etc.).")
	}

	// --- Payment processors (Braintree, etc.) ---
	if key := os.Getenv("BRAINTREE_PUBLIC_KEY"); key != "" {
		if bt, err := processor.Get(processor.Braintree); err == nil {
			if p, ok := bt.(*btprovider.Provider); ok {
				p.Configure(btprovider.Config{
					PublicKey:   key,
					PrivateKey:  os.Getenv("BRAINTREE_PRIVATE_KEY"),
					MerchantID:  os.Getenv("BRAINTREE_MERCHANT_ID"),
					Environment: envOr("BRAINTREE_ENV", "sandbox"),
				})
				log.Info().Msg("Braintree payment processor configured")
			}
		}
	}

	fundingSvc := funding.New()

	srv := api.NewServer(registry, listenAddr)
	srv.SetFunding(fundingSvc)

	// --- Admin Users ---
	adminStore := srv.AdminStore()
	adminUser := envOr("ADMIN_USERNAME", "admin")
	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminPass != "" {
		adminStore.AddAdmin(adminUser, adminPass, "super_admin")
		log.Info().Str("user", adminUser).Msg("Admin user configured")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", listenAddr).Strs("providers", registry.List()).Msg("Broker API starting")
		errCh <- srv.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("Shutting down...")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Shutdown error")
		}
	case err := <-errCh:
		if err != nil {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
