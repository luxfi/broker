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
	"github.com/luxfi/broker/pkg/provider/envconfig"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	listenAddr := envOr("BROKER_LISTEN", ":8090")

	registry := provider.NewRegistry()
	n := envconfig.RegisterFromEnv(registry)
	if n == 0 {
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
