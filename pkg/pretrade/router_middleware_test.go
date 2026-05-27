package pretrade

import (
	"context"
	"errors"
	"testing"
)

type fakeReq struct {
	accountID  string
	investorID string
	offeringID string
	side       string
	qty        string
}

type fakeResp struct {
	id string
}

func enrich(_ context.Context, accountID string, raw any) (*Order, error) {
	r, ok := raw.(*fakeReq)
	if !ok {
		return nil, errors.New("bad req")
	}
	return &Order{
		AccountID:  accountID,
		InvestorID: r.investorID,
		OfferingID: r.offeringID,
		Side:       r.side,
		Qty:        r.qty,
		Symbol:     "OSAGE",
	}, nil
}

func TestWrap_AllowDelegates(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	g, _ := newGateT(t, c, tx, a, k, ch)
	called := false
	inner := SubmitFunc[*fakeReq, *fakeResp](func(_ context.Context, _ string, _ *fakeReq) (*fakeResp, error) {
		called = true
		return &fakeResp{id: "abc"}, nil
	})
	w := Wrap(g, enrich, inner)
	resp, err := w(context.Background(), "acct1", &fakeReq{investorID: "inv1", offeringID: "off1", side: "buy", qty: "1"})
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if !called {
		t.Fatal("inner was not called")
	}
	if resp == nil || resp.id != "abc" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestWrap_DenyShortCircuits(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	k.s.Verified = false
	g, _ := newGateT(t, c, tx, a, k, ch)
	called := false
	inner := SubmitFunc[*fakeReq, *fakeResp](func(_ context.Context, _ string, _ *fakeReq) (*fakeResp, error) {
		called = true
		return &fakeResp{id: "abc"}, nil
	})
	w := Wrap(g, enrich, inner)
	_, err := w(context.Background(), "acct1", &fakeReq{investorID: "inv1", offeringID: "off1", side: "buy", qty: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if called {
		t.Fatal("inner should not have been called")
	}
	ge, ok := IsGateError(err)
	if !ok {
		t.Fatalf("expected GateError, got %v", err)
	}
	if !ge.IsDeny() {
		t.Fatalf("expected deny, got %+v", ge.Decision)
	}
}

func TestWrap_EscalateShortCircuits(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	c.suit.Ambiguous = true
	g, _ := newGateT(t, c, tx, a, k, ch)
	called := false
	inner := SubmitFunc[*fakeReq, *fakeResp](func(_ context.Context, _ string, _ *fakeReq) (*fakeResp, error) {
		called = true
		return &fakeResp{id: "abc"}, nil
	})
	w := Wrap(g, enrich, inner)
	_, err := w(context.Background(), "acct1", &fakeReq{investorID: "inv1", offeringID: "off1", side: "buy", qty: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if called {
		t.Fatal("inner should not have been called")
	}
	ge, ok := IsGateError(err)
	if !ok {
		t.Fatalf("expected GateError, got %v", err)
	}
	if !ge.IsEscalate() {
		t.Fatalf("expected escalate, got %+v", ge.Decision)
	}
}
