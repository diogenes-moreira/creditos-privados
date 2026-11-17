package odoo

import (
	"context"
	"sync"
	"time"
)

// limiter spaces requests to Odoo client-side. Odoo allows 60 requests a
// minute and 600 an hour per IP, and CuppoGan's service shares the egress IP,
// so Prestia stays well below with sliding windows of its own.
type limiter struct {
	mu      sync.Mutex
	now     func() time.Time
	windows []*window
}

type window struct {
	size   time.Duration
	max    int
	events []time.Time
}

func newLimiter(perMinute, perHour int) *limiter {
	l := &limiter{now: time.Now}
	if perMinute > 0 {
		l.windows = append(l.windows, &window{size: time.Minute, max: perMinute})
	}
	if perHour > 0 {
		l.windows = append(l.windows, &window{size: time.Hour, max: perHour})
	}
	return l
}

// Wait blocks until every window has room, then takes it.
func (l *limiter) Wait(ctx context.Context) error {
	for {
		wait := l.reserve()
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// reserve takes a slot and returns 0, or returns how long until one frees.
func (l *limiter) reserve() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	var wait time.Duration
	for _, w := range l.windows {
		w.prune(now)
		if len(w.events) >= w.max {
			if d := w.events[0].Add(w.size).Sub(now); d > wait {
				wait = d
			}
		}
	}
	if wait > 0 {
		return wait
	}
	for _, w := range l.windows {
		w.events = append(w.events, now)
	}
	return 0
}

// Remaining is the room left in the longest window (the hourly budget). The
// short window only paces requests; the long one is what a sync round can
// exhaust.
func (l *limiter) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.windows) == 0 {
		return 1 << 30
	}
	w := l.windows[len(l.windows)-1]
	w.prune(l.now())
	return w.max - len(w.events)
}

func (w *window) prune(now time.Time) {
	cut := 0
	for cut < len(w.events) && !w.events[cut].After(now.Add(-w.size)) {
		cut++
	}
	w.events = w.events[cut:]
}
