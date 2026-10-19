package publishproto

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNegotiateCurrentWindow(t *testing.T) {
	server := DefaultWindow()
	got := Negotiate(server, Offer{Min: Min, Max: Min, Protocol: Min})
	if !got.OK || got.Selected != Min {
		t.Fatalf("%+v", got)
	}
}

func TestNegotiateDegenerateOldPublisherIsTooOld(t *testing.T) {
	got := Negotiate(DefaultWindow(), Offer{Min: 0, Max: 0, Protocol: 0})
	if got.OK || got.Error != ErrUpgradeRequired {
		t.Fatalf("%+v", got)
	}
}

func TestNegotiateFutureProtocolIsTooNew(t *testing.T) {
	got := Negotiate(DefaultWindow(), Offer{Min: 99, Max: 99, Protocol: 99})
	if got.OK || got.Error != ErrTooNew {
		t.Fatalf("%+v", got)
	}
}

func TestNegotiateOverlappingWindowSelectsCurrent(t *testing.T) {
	got := Negotiate(Window{Min: Min, Max: Current}, Offer{Min: Min, Max: Current + 1, Protocol: Current})
	if !got.OK || got.Selected != Current {
		t.Fatalf("selected=%d want current %d: %+v", got.Selected, Current, got)
	}
}

func TestParseOfferTreatsBareProtocolAsDegenerateWindow(t *testing.T) {
	h := http.Header{}
	h.Set(ProtocolHeader, "2")
	offer := ParseOffer(h)
	if offer.Min != 2 || offer.Max != 2 || offer.Protocol != 2 {
		t.Fatalf("%+v", offer)
	}
}

func TestCheckRejectsTooNew(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(ProtocolHeader, "99")
	rec := httptest.NewRecorder()
	if Check(rec, req, DefaultWindow(), "test") {
		t.Fatal("expected rejection")
	}
	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("status=%d", rec.Code)
	}
	var decision Decision
	if err := json.NewDecoder(rec.Body).Decode(&decision); err != nil {
		t.Fatal(err)
	}
	if decision.Error != ErrTooNew {
		t.Fatalf("%+v", decision)
	}
}

func TestDisabledWindowAcceptsAnything(t *testing.T) {
	got := Negotiate(Window{Min: 0, Max: 0}, Offer{Protocol: 0})
	if !got.OK {
		t.Fatalf("%+v", got)
	}
}

// Write endpoints report the actionable reason in "cause" and may leave
// "message" empty. Explain must surface it instead of printing an empty tail.
func TestExplainFallsBackToCause(t *testing.T) {
	body := []byte(`{"ok":false,"code":"release.verify_failed","error":"release.verify_failed","cause":"manifest 0.1.0 does not list this backend's URL"}`)
	err := Explain(http.StatusConflict, body)
	if err == nil {
		t.Fatal("a non-OK decision with an error code must produce an error")
	}
	if got := err.Error(); got != "release.verify_failed: manifest 0.1.0 does not list this backend's URL" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestExplainPrefersMessageOverCause(t *testing.T) {
	body := []byte(`{"ok":false,"error":"release.verify_failed","message":"short reason","cause":"long detail"}`)
	err := Explain(http.StatusConflict, body)
	if err == nil || err.Error() != "release.verify_failed: short reason" {
		t.Fatalf("unexpected message: %v", err)
	}
}
