package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	messagecontentapp "app/internal/application/messagecontent"
	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
	nethtml "golang.org/x/net/html"
)

const (
	defaultMessageHistoryLimit   = 20
	maxMessageHistoryLimit       = 20
	maxClientMessageIDLength     = 128
	maxLinkPreviewReadBytes      = 1024
	maxMessageMentionTargets     = 50
	maxCardDescription           = 2000
	maxCardTitleLength           = 256
	maxCreateMessageRequestBytes = maxChartMessageBodyBytes + 1024
	linkPreviewFetchTimeout      = 2 * time.Second
	linkPreviewMaxRedirects      = 3
	messageTypeLink              = "link"
	messageTypeMarkdown          = "markdown"
	messageTypeCard              = "card"
	messageTypeText              = "text"
)

var messageMentionTokenPattern = regexp.MustCompile(`\{\(@(?:(user)/(all)|(user|app)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))\)\}`)

var (
	errConversationAccessDenied = errors.New("conversation access denied")
	errConversationNotSendable  = errors.New("conversation not sendable")
	errReplyToMessageInvalid    = errors.New("reply_to_message_id invalid")
)

type createMessageRequest struct {
	ClientMessageID  string          `json:"client_message_id" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	ReplyToMessageID string          `json:"reply_to_message_id,omitempty" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	Body             json.RawMessage `json:"body" swaggertype:"object"`
}

type messageSenderResponse struct {
	ID   string `json:"id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Type string `json:"type" example:"user"`
}

type messageDelegatedByResponse struct {
	ID   string `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name string `json:"name" example:"茉莉"`
	Type string `json:"type" example:"app"`
}

type messageReplyToSenderResponse struct {
	ID   string `json:"id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name string `json:"name" example:"Alice"`
	Type string `json:"type" example:"user"`
}

type messageReplyToResponse struct {
	ID      string                       `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Sender  messageReplyToSenderResponse `json:"sender"`
	Seq     int64                        `json:"seq" example:"12"`
	Summary string                       `json:"summary" example:"上一条消息摘要"`
}

type messageResponse struct {
	ClientMessageID  string                      `json:"client_message_id" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	Body             json.RawMessage             `json:"body,omitempty" swaggertype:"object"`
	ConversationID   string                      `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	CreatedAt        time.Time                   `json:"created_at" format:"date-time"`
	DelegatedBy      *messageDelegatedByResponse `json:"delegated_by,omitempty"`
	ID               string                      `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	ReplyToMessageID string                      `json:"reply_to_message_id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	ReplyTo          *messageReplyToResponse     `json:"reply_to,omitempty"`
	RevokedAt        *time.Time                  `json:"revoked_at,omitempty" format:"date-time"`
	RevokedByUserID  string                      `json:"revoked_by_user_id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Sender           messageSenderResponse       `json:"sender"`
	Seq              int64                       `json:"seq" example:"13"`
}

type createMessageResponse struct {
	Message messageResponse `json:"message"`
}

type listMessagesPageResponse struct {
	HasMoreAfter  bool  `json:"has_more_after" example:"false"`
	HasMoreBefore bool  `json:"has_more_before" example:"true"`
	Limit         int   `json:"limit" example:"20"`
	NewestSeq     int64 `json:"newest_seq" example:"120"`
	OldestSeq     int64 `json:"oldest_seq" example:"101"`
}

type listConversationMessagesResponse struct {
	Messages []messageResponse        `json:"messages"`
	Page     listMessagesPageResponse `json:"page"`
}

type listConversationMessagesQuery struct {
	AfterSeq  *int64
	BeforeSeq *int64
	Limit     int
}

type textMessageBody struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type markdownMessageBody struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type cardMessageBody struct {
	Description string `json:"description"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	URL         string `json:"url"`
}

type messageBodyEnvelope struct {
	Type string `json:"type"`
}

type finalizeMessageBodyFunc func(ctx context.Context, body json.RawMessage) (json.RawMessage, string, error)

type createMessageMetadata struct {
	DelegatedByType              *string
	DelegatedByID                *string
	DelegatedByName              string
	ReplyToMessageID             *string
	EmitAppEvent                 bool
	AfterCommitBeforeAppDelivery func(store.Message, []string, []string)
}

type messageMentionTarget struct {
	All        bool
	MemberID   string
	MemberType string
}

type cardMessageBodyHandler struct{}

var linkPreviewHTTPClient = newLinkPreviewHTTPClient()
var fetchLinkPreviewTitle = fetchLinkPreviewTitleHTTP

func normalizeMessageConversationID(rawID string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return "", errors.New("会话 ID 不能为空")
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("会话 ID 格式错误")
	}

	return parsedID.String(), nil
}

func normalizeListConversationMessagesQuery(rawBeforeSeq string, rawAfterSeq string, rawLimit string) (listConversationMessagesQuery, error) {
	beforeSeq, err := normalizeOptionalPositiveInt64(rawBeforeSeq, "before_seq")
	if err != nil {
		return listConversationMessagesQuery{}, err
	}
	afterSeq, err := normalizeOptionalPositiveInt64(rawAfterSeq, "after_seq")
	if err != nil {
		return listConversationMessagesQuery{}, err
	}
	if beforeSeq != nil && afterSeq != nil {
		return listConversationMessagesQuery{}, errors.New("before_seq 和 after_seq 不能同时传")
	}

	limit := defaultMessageHistoryLimit
	if strings.TrimSpace(rawLimit) != "" {
		parsedLimit, err := strconv.Atoi(strings.TrimSpace(rawLimit))
		if err != nil || parsedLimit <= 0 {
			return listConversationMessagesQuery{}, errors.New("limit 必须是正整数")
		}
		limit = parsedLimit
	}
	if limit > maxMessageHistoryLimit {
		limit = maxMessageHistoryLimit
	}

	return listConversationMessagesQuery{
		AfterSeq:  afterSeq,
		BeforeSeq: beforeSeq,
		Limit:     limit,
	}, nil
}

func normalizeOptionalPositiveInt64(rawValue string, fieldName string) (*int64, error) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return nil, nil
	}

	parsedValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsedValue <= 0 {
		return nil, errors.New(fieldName + " 必须是正整数")
	}

	return &parsedValue, nil
}

func normalizeOptionalMessageID(rawID string, fieldName string) (*string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return nil, nil
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.New(fieldName + " 格式错误")
	}
	normalizedID := parsedID.String()

	return &normalizedID, nil
}

func parseMessageMentionTargets(body json.RawMessage) []messageMentionTarget {
	content, ok := messageMentionContent(body)
	if !ok {
		return nil
	}

	matches := messageMentionTokenPattern.FindAllStringSubmatch(content, -1)
	targets := make([]messageMentionTarget, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) != 5 {
			continue
		}
		if match[2] == "all" {
			if _, ok := seen["all"]; ok {
				continue
			}
			seen["all"] = struct{}{}
			targets = append(targets, messageMentionTarget{All: true})
			if len(targets) >= maxMessageMentionTargets {
				break
			}
			continue
		}
		memberType := match[3]
		memberID, err := uuid.Parse(match[4])
		if err != nil {
			continue
		}
		target := messageMentionTarget{
			MemberID:   memberID.String(),
			MemberType: memberType,
		}
		key := conversationMemberMentionKey(target.MemberType, target.MemberID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
		if len(targets) >= maxMessageMentionTargets {
			break
		}
	}

	return targets
}

func messageMentionContent(body json.RawMessage) (string, bool) {
	var envelope messageBodyEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", false
	}

	switch strings.TrimSpace(envelope.Type) {
	case messageTypeText:
		var value textMessageBody
		if json.Unmarshal(body, &value) != nil {
			return "", false
		}
		return value.Content, true
	case messageTypeMarkdown:
		var value markdownMessageBody
		if json.Unmarshal(body, &value) != nil {
			return "", false
		}
		return value.Content, true
	default:
		return "", false
	}
}

func conversationMemberMentionKey(memberType string, memberID string) string {
	return memberType + "/" + memberID
}

func staticMessageBodyFinalizer(summary string) finalizeMessageBodyFunc {
	return func(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
		return body, summary, nil
	}
}

func (cardMessageBodyHandler) Type() string {
	return messageTypeCard
}

func (cardMessageBodyHandler) Validate(raw json.RawMessage) error {
	_, err := messagecontentapp.NewService(messagecontentapp.Dependencies{}).Normalize(context.Background(), raw)
	return err
}

func (cardMessageBodyHandler) Normalize(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	return messagecontentapp.NewService(messagecontentapp.Dependencies{}).Normalize(ctx, raw)
}

func (cardMessageBodyHandler) Summary(raw json.RawMessage) (string, error) {
	_, summary, err := messagecontentapp.NewService(messagecontentapp.Dependencies{}).
		Finalize(context.Background(), raw)
	return summary, err
}

func decodeCardMessageBody(raw json.RawMessage) (cardMessageBody, error) {
	var body cardMessageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return cardMessageBody{}, errors.New("消息体格式错误")
	}

	return body, nil
}

func fetchLinkPreviewTitleHTTP(ctx context.Context, linkURL string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, linkPreviewFetchTimeout)
	defer cancel()

	parsedURL, err := url.Parse(linkURL)
	if err != nil {
		return "", err
	}
	if err := validateLinkFetchURL(requestCtx, parsedURL); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, linkURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", "MagicChat Link Preview")

	resp, err := linkPreviewHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.New("link preview response status is not successful")
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxLinkPreviewReadBytes))
	if err != nil {
		return "", err
	}

	return extractHTMLTitle(content), nil
}

func newLinkPreviewHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: linkPreviewFetchTimeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		return dialLinkPreviewAddress(ctx, dialer, network, address)
	}

	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= linkPreviewMaxRedirects {
				return errors.New("link preview redirect limit exceeded")
			}

			return validateLinkFetchURL(req.Context(), req.URL)
		},
		Timeout:   linkPreviewFetchTimeout,
		Transport: transport,
	}
}

func dialLinkPreviewAddress(ctx context.Context, dialer *net.Dialer, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	addrs, err := resolveAllowedLinkFetchAddrs(ctx, host)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, addr := range addrs {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}

	return nil, errors.New("link preview host has no address")
}

func validateLinkFetchURL(ctx context.Context, parsedURL *url.URL) error {
	if parsedURL == nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return errors.New("link preview url is not allowed")
	}

	return validateLinkFetchHost(ctx, parsedURL.Hostname())
}

func validateLinkFetchHost(ctx context.Context, host string) error {
	_, err := resolveAllowedLinkFetchAddrs(ctx, host)

	return err
}

func resolveAllowedLinkFetchAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	normalizedHost := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if normalizedHost == "" || normalizedHost == "localhost" || strings.HasSuffix(normalizedHost, ".localhost") {
		return nil, errors.New("link preview host is not allowed")
	}

	if addr, err := netip.ParseAddr(normalizedHost); err == nil {
		if err := validateLinkFetchAddr(addr); err != nil {
			return nil, err
		}

		return []netip.Addr{addr.Unmap()}, nil
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, normalizedHost)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("link preview host has no address")
	}
	addrs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			return nil, errors.New("link preview host has invalid address")
		}
		if err := validateLinkFetchAddr(addr); err != nil {
			return nil, err
		}
		addrs = append(addrs, addr.Unmap())
	}

	return addrs, nil
}

func validateLinkFetchAddr(addr netip.Addr) error {
	addr = addr.Unmap()
	if !addr.IsValid() ||
		addr.IsUnspecified() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsMulticast() ||
		linkPreviewCarrierGradeNATPrefix().Contains(addr) {
		return errors.New("link preview address is not allowed")
	}

	return nil
}

func linkPreviewCarrierGradeNATPrefix() netip.Prefix {
	return netip.MustParsePrefix("100.64.0.0/10")
}

func extractHTMLTitle(content []byte) string {
	document, err := nethtml.Parse(bytes.NewReader(content))
	if err != nil {
		return ""
	}

	return normalizeLinkMessageTitle(findHTMLTitleText(document))
}

func findHTMLTitleText(node *nethtml.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == nethtml.ElementNode && strings.EqualFold(node.Data, "title") {
		var buffer strings.Builder
		collectHTMLText(node, &buffer)
		return buffer.String()
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if title := findHTMLTitleText(child); title != "" {
			return title
		}
	}

	return ""
}

func collectHTMLText(node *nethtml.Node, buffer *strings.Builder) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.TextNode {
			buffer.WriteString(child.Data)
		}
		collectHTMLText(child, buffer)
	}
}

func normalizeLinkMessageTitle(title string) string {
	return strings.Join(strings.Fields(stdhtml.UnescapeString(title)), " ")
}

func realtimeMessageCreatedEvent(message messageResponse) realtime.Envelope {
	return realtime.NewEvent(realtime.EventMessageCreated, createMessageResponse{
		Message: message,
	})
}

func realtimeMessageUpdatedEvent(message messageResponse) realtime.Envelope {
	return realtime.NewEvent(realtime.EventMessageUpdated, createMessageResponse{
		Message: message,
	})
}

type conversationRemovedEventPayload struct {
	ConversationID string `json:"conversation_id"`
}

type conversationMemberMentionedEventPayload struct {
	ConversationID   string `json:"conversation_id"`
	LastMentionedSeq int64  `json:"last_mentioned_seq"`
}

func realtimeConversationRemovedEvent(conversationID string) realtime.Envelope {
	return realtime.NewEvent(realtime.EventConversationRemoved, conversationRemovedEventPayload{
		ConversationID: conversationID,
	})
}

func realtimeConversationMemberMentionedEvent(conversationID string, lastMentionedSeq int64) realtime.Envelope {
	return realtime.NewEvent(realtime.EventMemberMentioned, conversationMemberMentionedEventPayload{
		ConversationID:   conversationID,
		LastMentionedSeq: lastMentionedSeq,
	})
}
