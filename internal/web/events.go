package web

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HellRok/loom/internal/store"
)

// keepAlive is how often an idle stream gets a comment, so proxies and the
// browser don't give up on it.
const keepAlive = 20 * time.Second

// streamBuffer is how many writes a slow client may fall behind by before
// its stream is cut. It then reconnects and refetches, which is how a
// client recovers from anything it missed: there is no Event replay.
const streamBuffer = 64

// broker fans committed writes out to the open SSE streams: one per Project
// (/<slug>/events) and one for home (/events), which sees every Project.
type broker struct {
	mu     sync.Mutex
	subs   map[*subscriber]struct{}
	closed bool
}

type subscriber struct {
	slug string // "" for home
	ch   chan []byte
}

func newBroker() *broker { return &broker{subs: map[*subscriber]struct{}{}} }

// subscribe opens a stream for slug ("" is home). It returns nil once the
// broker is closed.
func (b *broker) subscribe(slug string) *subscriber {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	s := &subscriber{slug: slug, ch: make(chan []byte, streamBuffer)}
	b.subs[s] = struct{}{}
	return s
}

func (b *broker) unsubscribe(s *subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.drop(s)
}

// drop ends s's stream; b.mu must be held.
func (b *broker) drop(s *subscriber) {
	if _, ok := b.subs[s]; ok {
		delete(b.subs, s)
		close(s.ch)
	}
}

// close ends every stream and refuses new ones, so shutdown needn't wait
// for clients to hang up.
func (b *broker) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for s := range b.subs {
		b.drop(s)
	}
}

// publish sends c to the streams it concerns: an issue-<n> event for each
// changed Issue, then changed. It never blocks; a client that has fallen
// too far behind is cut off.
func (b *broker) publish(c store.Change) {
	var frame strings.Builder
	for _, n := range c.Issues {
		fmt.Fprintf(&frame, "event: issue-%d\ndata: %s#%d\n\n", n, c.Slug, n)
	}
	fmt.Fprintf(&frame, "event: changed\ndata: %s\n\n", c.Slug)
	msg := []byte(frame.String())

	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if s.slug != "" && s.slug != c.Slug {
			continue
		}
		select {
		case s.ch <- msg:
		default:
			b.drop(s)
		}
	}
}

// serve streams events to the client until it hangs up or the broker closes.
func (b *broker) serve(w http.ResponseWriter, r *http.Request, slug string) {
	rc := http.NewResponseController(w)
	sub := b.subscribe(slug)
	if sub == nil {
		http.Error(w, "loom: shutting down", http.StatusServiceUnavailable)
		return
	}
	defer b.unsubscribe(sub)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// retry sets how soon the browser reconnects after the daemon restarts.
	if _, err := w.Write([]byte("retry: 2000\n\n")); err != nil || rc.Flush() != nil {
		return
	}

	tick := time.NewTicker(keepAlive)
	defer tick.Stop()
	for {
		select {
		case msg, ok := <-sub.ch:
			if !ok {
				return
			}
			if _, err := w.Write(msg); err != nil || rc.Flush() != nil {
				return
			}
		case <-tick.C:
			if _, err := w.Write([]byte(": keep-alive\n\n")); err != nil || rc.Flush() != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
