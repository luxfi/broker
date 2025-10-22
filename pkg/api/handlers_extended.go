package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// --- Account Management Handlers ---

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	am, ok := p.(provider.AccountManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support account updates")
		return
	}
	var req types.UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	acct, err := am.UpdateAccount(r.Context(), chi.URLParam(r, "accountId"), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (s *Server) handleCloseAccount(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	am, ok := p.(provider.AccountManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support account close")
		return
	}
	if err := am.CloseAccount(r.Context(), chi.URLParam(r, "accountId")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

func (s *Server) handleGetAccountActivities(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	am, ok := p.(provider.AccountManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support account activities")
		return
	}
	q := r.URL.Query()
	params := &types.ActivityParams{
		Date:      q.Get("date"),
		After:     q.Get("after"),
		Until:     q.Get("until"),
		Direction: q.Get("direction"),
		PageToken: q.Get("page_token"),
	}
	if at := q.Get("activity_type"); at != "" {
		params.ActivityTypes = strings.Split(at, ",")
	}
	if ps := q.Get("page_size"); ps != "" {
		params.PageSize, _ = strconv.Atoi(ps)
	}
	activities, err := am.GetAccountActivities(r.Context(), chi.URLParam(r, "accountId"), params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, activities)
}

// --- Document Handlers ---

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dm, ok := p.(provider.DocumentManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support documents")
		return
	}
	var doc types.DocumentUpload
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := dm.UploadDocument(r.Context(), chi.URLParam(r, "accountId"), &doc)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dm, ok := p.(provider.DocumentManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support documents")
		return
	}
	q := r.URL.Query()
	params := &types.DocumentParams{
		Start: q.Get("start"),
		End:   q.Get("end"),
	}
	docs, err := dm.ListDocuments(r.Context(), chi.URLParam(r, "accountId"), params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dm, ok := p.(provider.DocumentManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support documents")
		return
	}
	doc, err := dm.GetDocument(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "documentId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleDownloadDocument(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dm, ok := p.(provider.DocumentManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support documents")
		return
	}
	data, contentType, err := dm.DownloadDocument(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "documentId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// --- Journal Handlers ---

func (s *Server) handleCreateJournal(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jm, ok := p.(provider.JournalManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support journals")
		return
	}
	var req types.CreateJournalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	journal, err := jm.CreateJournal(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, journal)
}

func (s *Server) handleListJournals(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jm, ok := p.(provider.JournalManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support journals")
		return
	}
	q := r.URL.Query()
	params := &types.JournalParams{
		After:       q.Get("after"),
		Before:      q.Get("before"),
		Status:      q.Get("status"),
		EntryType:   q.Get("entry_type"),
		ToAccount:   q.Get("to_account"),
		FromAccount: q.Get("from_account"),
	}
	journals, err := jm.ListJournals(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, journals)
}

func (s *Server) handleGetJournal(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jm, ok := p.(provider.JournalManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support journals")
		return
	}
	journal, err := jm.GetJournal(r.Context(), chi.URLParam(r, "journalId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, journal)
}

func (s *Server) handleDeleteJournal(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jm, ok := p.(provider.JournalManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support journals")
		return
	}
	if err := jm.DeleteJournal(r.Context(), chi.URLParam(r, "journalId")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleCreateBatchJournal(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jm, ok := p.(provider.JournalManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support journals")
		return
	}
	var req types.BatchJournalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	journals, err := jm.CreateBatchJournal(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, journals)
}

func (s *Server) handleReverseBatchJournal(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jm, ok := p.(provider.JournalManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support journals")
		return
	}
	var req types.ReverseBatchJournalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	journals, err := jm.ReverseBatchJournal(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, journals)
}

// --- Transfer Extended Handlers ---

func (s *Server) handleCancelTransfer(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	te, ok := p.(provider.TransferExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support transfer cancellation")
		return
	}
	if err := te.CancelTransfer(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "transferId")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handleDeleteACHRelationship(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	te, ok := p.(provider.TransferExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support ACH deletion")
		return
	}
	if err := te.DeleteACHRelationship(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "achId")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleCreateRecipientBank(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	te, ok := p.(provider.TransferExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support wire transfers")
		return
	}
	var req types.CreateBankRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	bank, err := te.CreateRecipientBank(r.Context(), chi.URLParam(r, "accountId"), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, bank)
}

func (s *Server) handleListRecipientBanks(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	te, ok := p.(provider.TransferExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support wire transfers")
		return
	}
	banks, err := te.ListRecipientBanks(r.Context(), chi.URLParam(r, "accountId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, banks)
}

func (s *Server) handleDeleteRecipientBank(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	te, ok := p.(provider.TransferExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support wire transfers")
		return
	}
	if err := te.DeleteRecipientBank(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "bankId")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Crypto Data Handlers ---

func (s *Server) handleGetCryptoBars(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cp, ok := p.(provider.CryptoDataProvider)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support crypto data")
		return
	}
	q := r.URL.Query()
	req := &types.CryptoBarsRequest{
		Symbols:   strings.Split(q.Get("symbols"), ","),
		Timeframe: q.Get("timeframe"),
		Start:     q.Get("start"),
		End:       q.Get("end"),
		PageToken: q.Get("page_token"),
	}
	if l := q.Get("limit"); l != "" {
		req.Limit, _ = strconv.Atoi(l)
	}
	bars, err := cp.GetCryptoBars(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bars)
}

func (s *Server) handleGetCryptoQuotes(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cp, ok := p.(provider.CryptoDataProvider)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support crypto data")
		return
	}
	q := r.URL.Query()
	req := &types.CryptoQuotesRequest{
		Symbols:   strings.Split(q.Get("symbols"), ","),
		Start:     q.Get("start"),
		End:       q.Get("end"),
		PageToken: q.Get("page_token"),
	}
	if l := q.Get("limit"); l != "" {
		req.Limit, _ = strconv.Atoi(l)
	}
	quotes, err := cp.GetCryptoQuotes(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, quotes)
}

func (s *Server) handleGetCryptoTrades(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cp, ok := p.(provider.CryptoDataProvider)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support crypto data")
		return
	}
	q := r.URL.Query()
	req := &types.CryptoTradesRequest{
		Symbols:   strings.Split(q.Get("symbols"), ","),
		Start:     q.Get("start"),
		End:       q.Get("end"),
		PageToken: q.Get("page_token"),
	}
	if l := q.Get("limit"); l != "" {
		req.Limit, _ = strconv.Atoi(l)
	}
	trades, err := cp.GetCryptoTrades(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, trades)
}

func (s *Server) handleGetCryptoSnapshots(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cp, ok := p.(provider.CryptoDataProvider)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support crypto data")
		return
	}
	syms := r.URL.Query().Get("symbols")
	if syms == "" {
		writeError(w, http.StatusBadRequest, "symbols query param required")
		return
	}
	snaps, err := cp.GetCryptoSnapshots(r.Context(), strings.Split(syms, ","))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snaps)
}

// --- Portfolio History Handler ---

func (s *Server) handleGetPortfolioHistory(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pa, ok := p.(provider.PortfolioAnalyzer)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support portfolio history")
		return
	}
	q := r.URL.Query()
	params := &types.HistoryParams{
		Period:        q.Get("period"),
		Timeframe:     q.Get("timeframe"),
		DateEnd:       q.Get("date_end"),
		ExtendedHours: q.Get("extended_hours") == "true",
	}
	history, err := pa.GetPortfolioHistory(r.Context(), chi.URLParam(r, "accountId"), params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, history)
}

// --- Watchlist Handlers ---

func (s *Server) handleCreateWatchlist(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wm, ok := p.(provider.WatchlistManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support watchlists")
		return
	}
	var req types.CreateWatchlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	wl, err := wm.CreateWatchlist(r.Context(), chi.URLParam(r, "accountId"), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, wl)
}

func (s *Server) handleListWatchlists(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wm, ok := p.(provider.WatchlistManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support watchlists")
		return
	}
	watchlists, err := wm.ListWatchlists(r.Context(), chi.URLParam(r, "accountId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, watchlists)
}

func (s *Server) handleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wm, ok := p.(provider.WatchlistManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support watchlists")
		return
	}
	wl, err := wm.GetWatchlist(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "watchlistId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wl)
}

func (s *Server) handleUpdateWatchlist(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wm, ok := p.(provider.WatchlistManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support watchlists")
		return
	}
	var req types.UpdateWatchlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	wl, err := wm.UpdateWatchlist(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "watchlistId"), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wl)
}

func (s *Server) handleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wm, ok := p.(provider.WatchlistManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support watchlists")
		return
	}
	if err := wm.DeleteWatchlist(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "watchlistId")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAddWatchlistAsset(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wm, ok := p.(provider.WatchlistManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support watchlists")
		return
	}
	var req struct {
		Symbol string `json:"symbol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	wl, err := wm.AddWatchlistAsset(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "watchlistId"), req.Symbol)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wl)
}

func (s *Server) handleRemoveWatchlistAsset(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wm, ok := p.(provider.WatchlistManager)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support watchlists")
		return
	}
	if err := wm.RemoveWatchlistAsset(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "watchlistId"), chi.URLParam(r, "symbol")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// --- Event Streaming Handlers ---

func (s *Server) handleStreamTradeEvents(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	es, ok := p.(provider.EventStreamer)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support event streaming")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, err := es.StreamTradeEvents(r.Context(), r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			w.Write([]byte("event: trade\ndata: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func (s *Server) handleStreamAccountEvents(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	es, ok := p.(provider.EventStreamer)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support event streaming")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, err := es.StreamAccountEvents(r.Context(), r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			w.Write([]byte("event: account\ndata: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func (s *Server) handleStreamTransferEvents(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	es, ok := p.(provider.EventStreamer)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support event streaming")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, err := es.StreamTransferEvents(r.Context(), r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			w.Write([]byte("event: transfer\ndata: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func (s *Server) handleStreamJournalEvents(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	es, ok := p.(provider.EventStreamer)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support event streaming")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, err := es.StreamJournalEvents(r.Context(), r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			w.Write([]byte("event: journal\ndata: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
