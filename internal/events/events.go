// Package events defines the minimal publishing interface used by domain
// packages to emit real-time events without depending on the WebSocket layer.
package events

// Sink receives typed application events. The WebSocket hub implements it.
type Sink interface {
	Publish(eventType string, data any)
}

// Noop is a Sink that discards events (useful in tests).
type Noop struct{}

func (Noop) Publish(string, any) {}
