package tools_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSessionIsRequired(t *testing.T) {
	h := newHarness(t)
	if r := h.callWith(http.Header{}, "list_projects", nil); r.code() != "no_session" {
		t.Fatalf("want no_session, got %v", r)
	}
}

func TestSessionLength(t *testing.T) {
	h := newHarness(t)
	for _, s := range []string{strings.Repeat("é", 15), strings.Repeat("x", 257)} {
		hdr := http.Header{"X-Loom-Session": {s}}
		if r := h.callWith(hdr, "list_projects", nil); r.code() != "invalid_session" {
			t.Fatalf("%d runes: want invalid_session, got %v", len([]rune(s)), r)
		}
	}
	// Length is counted in runes, not bytes.
	for _, s := range []string{strings.Repeat("é", 16), strings.Repeat("x", 256)} {
		hdr := http.Header{"X-Loom-Session": {s}}
		h.mustOK(h.callWith(hdr, "list_projects", nil))
	}
}

func TestSessionFromArgumentForDirectHTTPClients(t *testing.T) {
	h := newHarness(t)
	h.mustOK(h.callWith(http.Header{}, "list_projects", map[string]any{"session": "direct-http-client-1"}))
}

func TestSessionHeaderWinsOverArgument(t *testing.T) {
	h := newHarness(t)
	hdr := http.Header{"X-Loom-Session": {"header-session-0001"}}
	// The invalid argument is ignored because the header is present.
	h.mustOK(h.callWith(hdr, "list_projects", map[string]any{"session": "short"}))
}

func TestSessionIsRecordedWithFirstAndLastSeen(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.mustOK(h.call("", "list_projects", nil))
	first, err := h.store.SessionByID(ctx, testSession)
	if err != nil {
		t.Fatalf("after first call: %v", err)
	}
	if !first.FirstSeen.Equal(first.LastSeen) {
		t.Fatalf("new Session: first_seen %v != last_seen %v", first.FirstSeen, first.LastSeen)
	}

	h.mustOK(h.call("", "list_projects", nil))
	again, err := h.store.SessionByID(ctx, testSession)
	if err != nil {
		t.Fatalf("after second call: %v", err)
	}
	if !again.FirstSeen.Equal(first.FirstSeen) || !again.LastSeen.After(first.LastSeen) {
		t.Fatalf("second call: got first %v last %v; want first %v and last after %v",
			again.FirstSeen, again.LastSeen, first.FirstSeen, first.LastSeen)
	}
}
