package appclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var errWebSocketUnavailable = errors.New("websocket unavailable")

type requestTransport interface {
	Send(context.Context, envelope) (<-chan struct{}, error)
}

type reliableRequesterOptions struct {
	MaxRetries   int
	ResponseWait time.Duration
	Sleep        func(context.Context, time.Duration) error
}

type reliableRequester struct {
	transport requestTransport

	maxRetries   int
	responseWait time.Duration
	sleep        func(context.Context, time.Duration) error

	mu      sync.Mutex
	pending map[string]chan envelope
}

func newReliableRequester(transport requestTransport, options reliableRequesterOptions) *reliableRequester {
	maxRetries := options.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 10
	}
	responseWait := options.ResponseWait
	if responseWait <= 0 {
		responseWait = requestWait
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return &reliableRequester{
		transport:    transport,
		maxRetries:   maxRetries,
		responseWait: responseWait,
		sleep:        sleep,
		pending:      make(map[string]chan envelope),
	}
}

func (r *reliableRequester) Request(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	content, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request := envelope{
		V:       protocolVersion,
		Kind:    kindRequest,
		ID:      newRequestID(),
		Method:  method,
		Payload: content,
	}
	return r.requestEnvelope(ctx, request)
}

func (r *reliableRequester) RequestEnvelope(ctx context.Context, request envelope) (json.RawMessage, error) {
	if request.ID == "" {
		request.ID = newRequestID()
	}
	if request.V == 0 {
		request.V = protocolVersion
	}
	if request.Kind == "" {
		request.Kind = kindRequest
	}
	return r.requestEnvelope(ctx, request)
}

func (r *reliableRequester) requestEnvelope(ctx context.Context, request envelope) (json.RawMessage, error) {
	if _, err := encodeEnvelope(request); err != nil {
		return nil, err
	}

	responseCh := make(chan envelope, 1)
	r.mu.Lock()
	r.pending[request.ID] = responseCh
	r.mu.Unlock()
	defer r.forget(request.ID)

	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, r.responseWait)
		generationDone, sendErr := r.transport.Send(attemptCtx, request)
		if sendErr == nil {
			response, waitErr := r.waitForResponse(attemptCtx, responseCh, generationDone)
			cancelAttempt()
			if waitErr == nil {
				if response.OK != nil && !*response.OK {
					if response.Error != nil {
						return nil, fmt.Errorf("%s: %s", response.Error.Code, response.Error.Message)
					}
					return nil, fmt.Errorf("app request failed")
				}
				return response.Payload, nil
			}
			lastErr = waitErr
		} else {
			cancelAttempt()
			lastErr = sendErr
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt == r.maxRetries {
			break
		}
		if err := r.sleep(ctx, retryDelay(attempt)); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("%w after %d retries: %v", errWebSocketUnavailable, r.maxRetries, lastErr)
}

func (r *reliableRequester) HandleResponse(response envelope) {
	r.mu.Lock()
	responseCh := r.pending[response.ReplyTo]
	r.mu.Unlock()
	if responseCh == nil {
		return
	}
	select {
	case responseCh <- response:
	default:
	}
}

func (r *reliableRequester) waitForResponse(ctx context.Context, responseCh <-chan envelope, generationDone <-chan struct{}) (envelope, error) {
	timer := time.NewTimer(r.responseWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return envelope{}, ctx.Err()
	case response := <-responseCh:
		return response, nil
	case <-generationDone:
		return envelope{}, errWebSocketUnavailable
	case <-timer.C:
		return envelope{}, fmt.Errorf("app request response timeout")
	}
}

func (r *reliableRequester) forget(id string) {
	r.mu.Lock()
	delete(r.pending, id)
	r.mu.Unlock()
}

func retryDelay(retry int) time.Duration {
	delay := time.Second << retry
	if delay > maxReconnectBackoff {
		return maxReconnectBackoff
	}
	return delay
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
