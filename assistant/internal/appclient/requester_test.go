package appclient

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type scriptedRequestTransport struct {
	onSend func(envelope, int) (<-chan struct{}, error)
	sent   []envelope
}

func (t *scriptedRequestTransport) Send(ctx context.Context, message envelope) (<-chan struct{}, error) {
	t.sent = append(t.sent, message)
	if t.onSend == nil {
		return nil, errors.New("send failed")
	}
	return t.onSend(message, len(t.sent))
}

func TestReliableRequesterRetriesWithStableRequestID(t *testing.T) {
	transport := &scriptedRequestTransport{}
	var requester *reliableRequester
	transport.onSend = func(message envelope, attempt int) (<-chan struct{}, error) {
		done := make(chan struct{})
		if attempt == 1 {
			close(done)
			return done, nil
		}
		ok := true
		requester.HandleResponse(envelope{
			V:       protocolVersion,
			Kind:    kindResponse,
			ReplyTo: message.ID,
			OK:      &ok,
			Payload: json.RawMessage(`{"retried":true}`),
		})
		return done, nil
	}
	delays := make([]time.Duration, 0)
	requester = newReliableRequester(transport, reliableRequesterOptions{
		MaxRetries: 10,
		Sleep: func(ctx context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	})

	raw, err := requester.Request(context.Background(), "test.method", map[string]any{"value": 1})
	if err != nil {
		t.Fatalf("Request() error = %v, want nil", err)
	}
	if string(raw) != `{"retried":true}` {
		t.Fatalf("Request() payload = %s, want retried response", raw)
	}
	if len(transport.sent) != 2 {
		t.Fatalf("send count = %d, want 2", len(transport.sent))
	}
	if transport.sent[0].ID == "" || transport.sent[0].ID != transport.sent[1].ID {
		t.Fatalf("request IDs = %q and %q, want same non-empty ID", transport.sent[0].ID, transport.sent[1].ID)
	}
	if len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("retry delays = %v, want [1s]", delays)
	}
}

func TestReliableRequesterStopsAfterTenRetries(t *testing.T) {
	transport := &scriptedRequestTransport{}
	transport.onSend = func(message envelope, attempt int) (<-chan struct{}, error) {
		return nil, errors.New("offline")
	}
	requester := newReliableRequester(transport, reliableRequesterOptions{
		MaxRetries: 10,
		Sleep: func(context.Context, time.Duration) error {
			return nil
		},
	})

	_, err := requester.Request(context.Background(), "test.method", map[string]any{})
	if err == nil || !errors.Is(err, errWebSocketUnavailable) {
		t.Fatalf("Request() error = %v, want websocket unavailable", err)
	}
	if len(transport.sent) != 11 {
		t.Fatalf("send count = %d, want initial attempt plus 10 retries", len(transport.sent))
	}
}

func TestReliableRequesterBoundsWaitingForDisconnectedTransport(t *testing.T) {
	blocking := requestTransportFunc(func(ctx context.Context, message envelope) (<-chan struct{}, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	requester := newReliableRequester(blocking, reliableRequesterOptions{
		MaxRetries:   2,
		ResponseWait: 5 * time.Millisecond,
		Sleep:        func(context.Context, time.Duration) error { return nil },
	})

	started := time.Now()
	_, err := requester.Request(context.Background(), "test.method", nil)
	if err == nil || !errors.Is(err, errWebSocketUnavailable) {
		t.Fatalf("Request() error = %v, want websocket unavailable", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Request() elapsed = %s, want bounded transport waits", elapsed)
	}
}

type requestTransportFunc func(context.Context, envelope) (<-chan struct{}, error)

func (f requestTransportFunc) Send(ctx context.Context, message envelope) (<-chan struct{}, error) {
	return f(ctx, message)
}

func TestReliableRequesterDoesNotRetryProtocolError(t *testing.T) {
	transport := &scriptedRequestTransport{}
	var requester *reliableRequester
	transport.onSend = func(message envelope, attempt int) (<-chan struct{}, error) {
		done := make(chan struct{})
		ok := false
		requester.HandleResponse(envelope{
			V:       protocolVersion,
			Kind:    kindResponse,
			ReplyTo: message.ID,
			OK:      &ok,
			Error:   &errorPayload{Code: "forbidden", Message: "denied"},
		})
		return done, nil
	}
	requester = newReliableRequester(transport, reliableRequesterOptions{
		MaxRetries: 10,
		Sleep:      func(context.Context, time.Duration) error { return nil },
	})

	_, err := requester.Request(context.Background(), "test.method", nil)
	if err == nil || err.Error() != "forbidden: denied" {
		t.Fatalf("Request() error = %v, want forbidden error", err)
	}
	if len(transport.sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(transport.sent))
	}
}
