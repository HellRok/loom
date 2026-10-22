package tools_test

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HellRok/loom/internal/web"
)

// stream is an open SSE connection read line by line.
type stream struct {
	t     *testing.T
	lines chan string
}

func openStream(t *testing.T, url string) *stream {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("%s: Content-Type = %q", url, ct)
	}
	s := &stream{t: t, lines: make(chan string, 64)}
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			s.lines <- sc.Text()
		}
		close(s.lines)
	}()
	return s
}

// events reads n named events.
func (s *stream) events(n int) []string {
	s.t.Helper()
	var got []string
	for len(got) < n {
		select {
		case l, ok := <-s.lines:
			if !ok {
				s.t.Fatalf("stream closed after %v", got)
			}
			if name, ok := strings.CutPrefix(l, "event: "); ok {
				got = append(got, name)
			}
		case <-time.After(5 * time.Second):
			s.t.Fatalf("timed out after %v, want %d events", got, n)
		}
	}
	return got
}

// TestWriteThroughToolPublishesToStreams is the one SSE test: a write
// through a tool handler reaches the Project's stream and the home stream.
func TestWriteThroughToolPublishesToStreams(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	parent := h.issue(dir, map[string]any{"title": "Spec"})
	child := h.issue(dir, map[string]any{"title": "Child", "parent": parent})

	mux := http.NewServeMux()
	web.Register(mux, web.Deps{Store: h.store})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	project := openStream(t, ts.URL+"/app/events")
	home := openStream(t, ts.URL+"/events")

	h.mustOK(h.call(dir, "comment", map[string]any{"ref": child, "body": "hello"}))

	want := []string{"issue-2", "issue-1", "changed"}
	if got := project.events(3); !slices.Equal(got, want) {
		t.Errorf("project stream = %v, want %v", got, want)
	}
	if got := home.events(3); !slices.Equal(got, want) {
		t.Errorf("home stream = %v, want %v", got, want)
	}
}
