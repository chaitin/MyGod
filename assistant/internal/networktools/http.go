package networktools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"assistant/internal/mcpclient"
	"assistant/internal/publicnet"
)

const (
	maxHTTPRequestBodyBytes  = 1024 * 1024
	maxHTTPResponseBodyBytes = 1024 * 1024
	maxHTTPURLLength         = 4096
	httpRequestTimeout       = 30 * time.Second
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type httpClientInput struct {
	Body    *string           `json:"body"`
	Headers map[string]string `json:"headers"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
}

type httpClientResult struct {
	Body       string              `json:"body"`
	Headers    map[string][]string `json:"headers"`
	StatusCode int                 `json:"status_code"`
	Truncated  bool                `json:"truncated"`
}

func newHTTPClient(guard *publicnet.Guard) *http.Client {
	transport := &http.Transport{
		DialContext:           guard.DialContext,
		DisableCompression:    false,
		ForceAttemptHTTP2:     true,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   2,
		ResponseHeaderTimeout: 15 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   httpRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (s *Source) callHTTPClient(ctx context.Context, raw json.RawMessage) (mcpclient.ToolResult, error) {
	var input httpClientInput
	if err := decodeStrictJSON(raw, &input); err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("parse http_client input: %w", err)
	}
	method, err := normalizeHTTPMethod(input.Method)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	normalizedURL, host, err := normalizeHTTPURL(input.URL)
	if err != nil {
		return mcpclient.ToolResult{}, err
	}
	if err := s.guard.ValidateHost(ctx, host); err != nil {
		return mcpclient.ToolResult{}, err
	}

	var body io.Reader
	if input.Body != nil {
		if len([]byte(*input.Body)) > maxHTTPRequestBodyBytes {
			return mcpclient.ToolResult{}, fmt.Errorf("HTTP request body exceeds %d bytes", maxHTTPRequestBodyBytes)
		}
		body = strings.NewReader(*input.Body)
	}
	requestCtx, cancel := context.WithTimeout(ctx, httpRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, normalizedURL, body)
	if err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("create HTTP request: %w", err)
	}
	if err := applyHTTPHeaders(request, input.Headers); err != nil {
		return mcpclient.ToolResult{}, err
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("send HTTP request: %w", err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBodyBytes+1))
	if err != nil {
		return mcpclient.ToolResult{}, fmt.Errorf("read HTTP response: %w", err)
	}
	truncated := len(content) > maxHTTPResponseBodyBytes
	if truncated {
		content = content[:maxHTTPResponseBodyBytes]
	}

	return jsonResult(httpClientResult{
		Body:       string(content),
		Headers:    response.Header.Clone(),
		StatusCode: response.StatusCode,
		Truncated:  truncated,
	})
}

func normalizeHTTPMethod(raw string) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(raw))
	if method == "" {
		return "", fmt.Errorf("HTTP method is required")
	}
	for _, character := range method {
		if !isHTTPTokenRune(character) {
			return "", fmt.Errorf("HTTP method is invalid")
		}
	}
	if method == http.MethodConnect || method == http.MethodTrace {
		return "", fmt.Errorf("HTTP method %s is not allowed", method)
	}
	return method, nil
}

func isHTTPTokenRune(character rune) bool {
	if character > utf8.RuneSelf {
		return false
	}
	if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' {
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", character)
}

func normalizeHTTPURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxHTTPURLLength {
		return "", "", fmt.Errorf("HTTP URL is required and must not exceed %d characters", maxHTTPURLLength)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", fmt.Errorf("parse HTTP URL: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("HTTP URL scheme must be http or https")
	}
	if parsed.Opaque != "" || parsed.Host == "" || parsed.Hostname() == "" {
		return "", "", fmt.Errorf("HTTP URL must be absolute and include a host")
	}
	if parsed.User != nil {
		return "", "", fmt.Errorf("HTTP URL userinfo is not allowed; use headers instead")
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return "", "", fmt.Errorf("HTTP URL port is invalid")
		}
	}
	parsed.Fragment = ""
	return parsed.String(), parsed.Hostname(), nil
}

func applyHTTPHeaders(request *http.Request, headers map[string]string) error {
	for rawName, value := range headers {
		name := http.CanonicalHeaderKey(strings.TrimSpace(rawName))
		if !validHTTPHeaderName(name) {
			return fmt.Errorf("HTTP header name %q is invalid", rawName)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("HTTP header %q contains a newline", name)
		}
		switch strings.ToLower(name) {
		case "content-length":
			continue
		case "connection", "host", "keep-alive", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
			return fmt.Errorf("HTTP header %q is managed by the client and cannot be set", name)
		default:
			request.Header.Set(name, value)
		}
	}
	return nil
}

func validHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if !isHTTPTokenRune(character) {
			return false
		}
	}
	return true
}

func decodeStrictJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("multiple JSON values are not allowed")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func httpClientInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"method", "url"},
		"properties": map[string]any{
			"method": map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
			"url":    map[string]any{"type": "string", "minLength": 1, "maxLength": maxHTTPURLLength},
			"headers": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"body": map[string]any{"type": "string", "maxLength": maxHTTPRequestBodyBytes},
		},
		"additionalProperties": false,
	}
}
