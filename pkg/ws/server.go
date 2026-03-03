package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/luxfi/broker/pkg/marketdata"
)

// Server provides WebSocket market data streaming to clients.
// Institutional clients can subscribe to real-time consolidated feeds.
type Server struct {
	feed    *marketdata.Feed
	clients map[*Client]bool
	mu      sync.RWMutex
}

// Client represents a connected WebSocket client.
type Client struct {
	conn    http.ResponseWriter
	flusher http.Flusher
	subs    map[string]bool // subscribed symbols
	mu      sync.Mutex
	done    chan struct{}
}

// Message is the envelope for all WebSocket messages.
type Message struct {
	Type    string      `json:"type"`    // subscribe, unsubscribe, ticker, bbo, error
	Channel string      `json:"channel"` // ticker, orderbook, trades
	Symbol  string      `json:"symbol,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Time    string      `json:"time"`
}

func NewServer(feed *marketdata.Feed) *Server {
	return &Server{
		feed:    feed,
		clients: make(map[*Client]bool),
	}
}

// HandleSSE serves Server-Sent Events for real-time market data.
// SSE is simpler than WebSocket and works through proxies/load balancers.
// Path: GET /api/v1/stream?symbols=AAPL,BTC/USD
func (s *Server) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	symbols := r.URL.Query()["symbols"]
	if len(symbols) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Subscribe to all requested symbols
	var unsubs []func()
	merged := make(chan *marketdata.Ticker, 64)

	for _, sym := range symbols {
		ch, unsub := s.feed.Subscribe(sym)
		unsubs = append(unsubs, unsub)
		go func() {
			for t := range ch {
				select {
				case merged <- t:
				default:
				}
			}
		}()
	}

	defer func() {
		for _, unsub := range unsubs {
			unsub()
		}
	}()

	// Send initial snapshots
	for _, sym := range symbols {
		t, err := s.feed.GetTicker(sym)
		if err == nil {
			s.sendSSE(w, flusher, "ticker", t)
		}
	}

	// Stream updates
	for {
		select {
		case <-ctx.Done():
			return
		case ticker := <-merged:
			s.sendSSE(w, flusher, "ticker", ticker)
		}
	}
}

func (s *Server) sendSSE(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	msg := Message{
		Type: event,
		Data: data,
		Time: time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	w.Write([]byte("event: " + event + "\n"))
	w.Write([]byte("data: "))
	w.Write(b)
	w.Write([]byte("\n\n"))
	flusher.Flush()
}

// HandleBBO returns the current best bid/offer for a symbol.
// Path: GET /api/v1/bbo/{symbol}
func (s *Server) HandleBBO(w http.ResponseWriter, r *http.Request) {
	// Symbol extracted by caller
}

// StartPoller starts a background goroutine that polls provider snapshots
// at the given interval and updates the consolidated feed.
func (s *Server) StartPoller(ctx context.Context, interval time.Duration, pollFn func(ctx context.Context)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pollFn(ctx)
			}
		}
	}()
}
