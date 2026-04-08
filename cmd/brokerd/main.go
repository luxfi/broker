package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/commerce/payment/processor"
	btprovider "github.com/hanzoai/commerce/payment/providers/braintree"

	"github.com/luxfi/broker/pkg/api"
	"github.com/luxfi/broker/pkg/compliance"
	"github.com/luxfi/broker/pkg/db"
	"github.com/luxfi/broker/pkg/funding"
	"github.com/luxfi/broker/pkg/marketdata"
	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/provider/envconfig"
	"github.com/luxfi/broker/pkg/router"
	"github.com/luxfi/broker/pkg/webhook"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	listenAddr := envOr("BROKER_LISTEN", ":8090")

	registry := provider.NewRegistry()
	n := envconfig.RegisterFromEnv(registry)
	if n == 0 {
		if os.Getenv("BROKER_ENV") == "development" {
			log.Warn().Msg("No providers configured — running in compliance-only mode")
		} else {
			log.Fatal().Msg("No providers configured. Set provider env vars (ALPACA_API_KEY, etc.).")
		}
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

	// SOR core components
	sor := router.New(registry)
	twapScheduler := router.NewTWAPScheduler(registry, sor)
	feed := marketdata.NewFeed()
	arbThresholdBps := 5.0
	arbDetector := marketdata.NewArbitrageDetector(feed, arbThresholdBps)

	srv := api.NewServer(registry, listenAddr)
	srv.SetFunding(fundingSvc)
	srv.SetTWAP(twapScheduler)
	srv.SetArbitrageDetector(arbDetector, arbThresholdBps)

	// --- Compliance (KYC, onboarding, funds, eSign, RBAC) ---
	var complianceStore compliance.ComplianceStore
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		pool, err := db.NewPostgresPool(context.Background(), dbURL)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
		}
		defer pool.Close()
		if err := db.RunMigrations(context.Background(), pool); err != nil {
			log.Fatal().Err(err).Msg("Failed to run database migrations")
		}
		log.Info().Msg("PostgreSQL connected and migrations applied")
		complianceStore = compliance.NewPostgresStore(pool)
	} else {
		complianceStore = compliance.NewMemoryStore()
		log.Info().Msg("Using in-memory compliance store (set DATABASE_URL for PostgreSQL)")
	}
	if os.Getenv("BROKER_ENV") == "development" {
		compliance.SeedStore(complianceStore)
		log.Info().Msg("Compliance store seeded with demo data")
	}
	scamDB := compliance.NewScamDB()
	srv.Mount("/compliance", compliance.NewRouter(complianceStore, compliance.WithScamDB(scamDB), compliance.WithRegistry(registry)))
	log.Info().Msg("Compliance routes mounted at /compliance")

	webhookStore := webhook.NewMemoryStore()
	srv.Mount("/webhooks", webhook.NewRouter(webhookStore))
	log.Info().Msg("Webhook routes mounted at /webhooks")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scamDB.StartBackgroundRefresh(ctx)

	// Background asset sync: refresh tradable assets from all providers hourly.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		syncAssets := func() {
			for _, name := range registry.List() {
				p, err := registry.Get(name)
				if err != nil {
					continue
				}
				assets, err := p.ListAssets(ctx, "")
				if err != nil {
					log.Error().Err(err).Str("provider", name).Msg("asset sync failed")
					continue
				}
				log.Info().Str("provider", name).Int("count", len(assets)).Msg("asset sync complete")
			}
		}
		syncAssets() // initial sync on startup
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncAssets()
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", listenAddr).Strs("providers", registry.List()).Msg("Broker REST API starting")
		errCh <- srv.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("Shutting down...")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("REST shutdown error")
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
