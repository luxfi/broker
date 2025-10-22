package alpaca

import (
	"github.com/luxfi/broker/pkg/provider"
)

// Compile-time assertions that *Provider implements all optional interfaces.
var (
	_ provider.Provider         = (*Provider)(nil)
	_ provider.AccountManager   = (*Provider)(nil)
	_ provider.DocumentManager  = (*Provider)(nil)
	_ provider.JournalManager   = (*Provider)(nil)
	_ provider.TransferExtended = (*Provider)(nil)
	_ provider.CryptoDataProvider = (*Provider)(nil)
	_ provider.EventStreamer    = (*Provider)(nil)
	_ provider.PortfolioAnalyzer = (*Provider)(nil)
	_ provider.WatchlistManager = (*Provider)(nil)
)
