package appconnection

import (
	"encoding/json"
	"strings"
	"testing"

	"app/internal/realtime"
)

func TestDefaultMaxMessageBytesIsOneMiB(t *testing.T) {
	manager := NewManager(Options{})
	if manager.maxMessageBytes != 1<<20 {
		t.Fatalf("maxMessageBytes = %d, want %d", manager.maxMessageBytes, 1<<20)
	}
}

func TestLimitOutboundEnvelopeReplacesOversizedResponse(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"content": strings.Repeat("x", 1<<20)})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	response := realtime.NewResponse("request-1", nil)
	response.Payload = payload

	limited, ok := limitOutboundEnvelope(response, 1<<20)
	if !ok {
		t.Fatal("limitOutboundEnvelope() ok = false, want replacement response")
	}
	if limited.Error == nil || limited.Error.Code != "response_too_large" {
		t.Fatalf("limited response = %#v, want response_too_large", limited)
	}
	if limited.ReplyTo != "request-1" {
		t.Fatalf("limited.ReplyTo = %q, want request-1", limited.ReplyTo)
	}
}

func TestLimitOutboundEnvelopeSkipsOversizedEvent(t *testing.T) {
	event := realtime.NewEvent("large.event", map[string]any{"content": strings.Repeat("x", 1<<20)})
	if _, ok := limitOutboundEnvelope(event, 1<<20); ok {
		t.Fatal("limitOutboundEnvelope() ok = true, want oversized event skipped")
	}
}
