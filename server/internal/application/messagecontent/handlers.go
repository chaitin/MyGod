package messagecontent

import (
	"context"
	"encoding/json"
	"errors"
	stdhtml "html"
	"net/url"
	"strconv"
	"strings"

	entitycardapp "app/internal/application/entitycard"
	messageapp "app/internal/application/message"
	"app/internal/messageformat"
)

const (
	maxTextLength        = 5000
	maxLinkURLLength     = 2048
	maxCardTitleLength   = 256
	maxCardDescription   = 2000
	messageBodyEmpty     = "消息体不能为空"
	messageBodyMalformed = "消息体格式错误"
)

func (s *Service) handlers() map[string]bodyHandler {
	return map[string]bodyHandler{
		TypeText: textHandler{}, TypeMarkdown: markdownHandler{}, TypeLink: linkHandler{fetchTitle: s.fetchLinkTitle},
		TypeCard: cardHandler{}, TypeChart: chartHandler{}, TypeChoice: choiceHandler{},
	}
}

func (s *Service) Prepare(ctx context.Context, accountID string, raw json.RawMessage) (json.RawMessage, error) {
	if isEntityCard(raw) {
		body, err := s.resolveEntityCard(ctx, accountID, raw)
		if err != nil {
			return nil, err
		}
		return body, nil
	}
	handler, err := s.findHandler(raw)
	if err != nil {
		return nil, messageapp.InvalidRequestError(err.Error(), err)
	}
	normalized, err := handler.Normalize(ctx, raw)
	if err != nil {
		return nil, messageapp.InvalidRequestError(err.Error(), err)
	}
	return normalized, nil
}

func (s *Service) Normalize(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	handler, err := s.findHandler(raw)
	if err != nil {
		return nil, err
	}
	return handler.Normalize(ctx, raw)
}

func (s *Service) Finalize(ctx context.Context, raw json.RawMessage) (json.RawMessage, string, error) {
	handler, err := s.findHandler(raw)
	if err != nil {
		return nil, "", err
	}
	if finalizer, ok := handler.(bodyFinalizer); ok {
		raw, err = finalizer.Finalize(ctx, raw)
		if err != nil {
			return nil, "", err
		}
	}
	summary, err := handler.Summary(raw)
	if err != nil {
		return nil, "", err
	}
	return raw, summary, nil
}

func (s *Service) findHandler(raw json.RawMessage) (bodyHandler, error) {
	if len(raw) == 0 {
		return nil, errors.New(messageBodyEmpty)
	}
	var value envelope
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New(messageBodyMalformed)
	}
	messageType := strings.TrimSpace(value.Type)
	if messageType == "" {
		return nil, errors.New("消息类型不能为空")
	}
	handler, ok := s.handlers()[messageType]
	if !ok {
		return nil, errors.New("不支持的消息类型")
	}
	return handler, nil
}

func isEntityCard(raw json.RawMessage) bool {
	var value envelope
	return json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value.Type) == TypeEntityCard
}

func (s *Service) resolveEntityCard(ctx context.Context, accountID string, raw json.RawMessage) (json.RawMessage, error) {
	var request entityCardBody
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, messageapp.InvalidRequestError(messageBodyMalformed, err)
	}
	if strings.TrimSpace(request.Type) != TypeEntityCard {
		return nil, messageapp.InvalidRequestError("消息类型错误", nil)
	}
	if s.entityCards == nil {
		return nil, messageapp.InternalError(errors.New("entity card resolver is required"))
	}
	card, err := s.entityCards.Resolve(ctx, entitycardapp.ResolveCommand{
		AccountID: accountID, EntityID: request.EntityID, EntityType: request.EntityType,
	})
	if err != nil {
		switch entitycardapp.ErrorCodeOf(err) {
		case entitycardapp.CodeInvalidRequest:
			return nil, messageapp.InvalidRequestError(entitycardapp.ErrorMessage(err), err)
		case entitycardapp.CodeNotFound:
			return nil, messageapp.NotFoundError(entitycardapp.ErrorMessage(err), err)
		default:
			return nil, messageapp.InternalError(err)
		}
	}
	encoded, err := json.Marshal(cardBody{Description: card.Description, Title: card.Title, Type: TypeCard, URL: card.URL})
	if err != nil {
		return nil, messageapp.InternalError(err)
	}
	normalized, err := (cardHandler{}).Normalize(ctx, encoded)
	if err != nil {
		return nil, messageapp.InternalError(err)
	}
	return normalized, nil
}

type textHandler struct{}

func (textHandler) Type() string { return TypeText }

func (h textHandler) Validate(raw json.RawMessage) error {
	var body textBody
	if json.Unmarshal(raw, &body) != nil {
		return errors.New(messageBodyMalformed)
	}
	return validateTextBody(h.Type(), body.Type, body.Content)
}

func (h textHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if err := h.Validate(raw); err != nil {
		return nil, err
	}
	var body textBody
	_ = json.Unmarshal(raw, &body)
	return json.Marshal(textBody{Type: h.Type(), Content: strings.TrimSpace(body.Content)})
}

func (textHandler) Summary(raw json.RawMessage) (string, error) {
	var body textBody
	if json.Unmarshal(raw, &body) != nil {
		return "", errors.New(messageBodyMalformed)
	}
	return strings.TrimSpace(body.Content), nil
}

type markdownHandler struct{}

func (markdownHandler) Type() string { return TypeMarkdown }

func (h markdownHandler) Validate(raw json.RawMessage) error {
	var body markdownBody
	if json.Unmarshal(raw, &body) != nil {
		return errors.New(messageBodyMalformed)
	}
	return validateTextBody(h.Type(), body.Type, body.Content)
}

func (h markdownHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if err := h.Validate(raw); err != nil {
		return nil, err
	}
	var body markdownBody
	_ = json.Unmarshal(raw, &body)
	return json.Marshal(markdownBody{Type: h.Type(), Content: strings.TrimSpace(body.Content)})
}

func (markdownHandler) Summary(raw json.RawMessage) (string, error) {
	var body markdownBody
	if json.Unmarshal(raw, &body) != nil {
		return "", errors.New(messageBodyMalformed)
	}
	return messageformat.MarkdownPlainText(strings.TrimSpace(body.Content))
}

func validateTextBody(expectedType, actualType, content string) error {
	if strings.TrimSpace(actualType) != expectedType {
		return errors.New("消息类型错误")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("消息内容不能为空")
	}
	if len([]rune(content)) > maxTextLength {
		return errors.New("消息内容不能超过 5000 个字符")
	}
	return nil
}

type linkHandler struct{ fetchTitle LinkTitleFetcher }

func (linkHandler) Type() string { return TypeLink }

func (h linkHandler) Validate(raw json.RawMessage) error {
	var body linkBody
	if json.Unmarshal(raw, &body) != nil {
		return errors.New(messageBodyMalformed)
	}
	if strings.TrimSpace(body.Type) != h.Type() {
		return errors.New("消息类型错误")
	}
	_, err := normalizeLinkURL(body.URL)
	return err
}

func (h linkHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if err := h.Validate(raw); err != nil {
		return nil, err
	}
	var body linkBody
	_ = json.Unmarshal(raw, &body)
	normalizedURL, err := normalizeLinkURL(body.URL)
	if err != nil {
		return nil, err
	}
	return json.Marshal(linkBody{Type: h.Type(), URL: normalizedURL})
}

func (h linkHandler) Finalize(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var body linkBody
	if json.Unmarshal(raw, &body) != nil {
		return nil, errors.New(messageBodyMalformed)
	}
	normalizedURL, err := normalizeLinkURL(body.URL)
	if err != nil {
		return nil, err
	}
	title := ""
	if h.fetchTitle != nil {
		if fetched, fetchErr := h.fetchTitle(ctx, normalizedURL); fetchErr == nil {
			title = normalizeLinkTitle(fetched)
		}
	}
	if title == "" {
		title = linkFallbackTitle(normalizedURL)
	}
	return json.Marshal(linkBody{Type: h.Type(), URL: normalizedURL, Title: title})
}

func (linkHandler) Summary(raw json.RawMessage) (string, error) {
	var body linkBody
	if json.Unmarshal(raw, &body) != nil {
		return "", errors.New(messageBodyMalformed)
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = linkFallbackTitle(strings.TrimSpace(body.URL))
	}
	return "[链接] " + title, nil
}

func normalizeLinkURL(rawURL string) (string, error) {
	linkURL := strings.TrimSpace(rawURL)
	if linkURL == "" {
		return "", errors.New("链接不能为空")
	}
	if len([]rune(linkURL)) > maxLinkURLLength {
		return "", errors.New("链接不能超过 2048 个字符")
	}
	if strings.ContainsAny(linkURL, " \t\r\n") {
		return "", errors.New("链接格式错误")
	}
	if strings.HasPrefix(strings.ToLower(linkURL), "www.") {
		linkURL = "https://" + linkURL
	}
	parsed, err := url.Parse(linkURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("链接格式错误")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("只支持 http 或 https 链接")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "", errors.New("链接格式错误")
	}
	return parsed.String(), nil
}

func normalizeLinkTitle(title string) string {
	return strings.Join(strings.Fields(stdhtml.UnescapeString(title)), " ")
}

func linkFallbackTitle(linkURL string) string {
	parsed, err := url.Parse(linkURL)
	if err != nil {
		return strings.TrimSpace(linkURL)
	}
	if host := strings.TrimSpace(parsed.Hostname()); host != "" {
		return host
	}
	return strings.TrimSpace(linkURL)
}

type cardHandler struct{}

func (cardHandler) Type() string { return TypeCard }

func (h cardHandler) Validate(raw json.RawMessage) error {
	var body cardBody
	if json.Unmarshal(raw, &body) != nil {
		return errors.New(messageBodyMalformed)
	}
	if strings.TrimSpace(body.Type) != h.Type() {
		return errors.New("消息类型错误")
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		return errors.New("卡片标题不能为空")
	}
	if len([]rune(title)) > maxCardTitleLength {
		return errors.New("卡片标题不能超过 " + strconv.Itoa(maxCardTitleLength) + " 个字符")
	}
	if len([]rune(strings.TrimSpace(body.Description))) > maxCardDescription {
		return errors.New("卡片说明不能超过 2000 个字符")
	}
	_, err := normalizeCardURL(body.URL)
	return err
}

func (h cardHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if err := h.Validate(raw); err != nil {
		return nil, err
	}
	var body cardBody
	_ = json.Unmarshal(raw, &body)
	normalizedURL, err := normalizeCardURL(body.URL)
	if err != nil {
		return nil, err
	}
	return json.Marshal(cardBody{
		Description: strings.TrimSpace(body.Description), Title: strings.TrimSpace(body.Title), Type: h.Type(), URL: normalizedURL,
	})
}

func (cardHandler) Summary(raw json.RawMessage) (string, error) {
	var body cardBody
	if json.Unmarshal(raw, &body) != nil {
		return "", errors.New(messageBodyMalformed)
	}
	return "[卡片] " + strings.TrimSpace(body.Title), nil
}

func normalizeCardURL(rawURL string) (string, error) {
	cardURL := strings.TrimSpace(rawURL)
	if cardURL == "" {
		return "", errors.New("链接不能为空")
	}
	if len([]rune(cardURL)) > maxLinkURLLength {
		return "", errors.New("链接不能超过 2048 个字符")
	}
	if strings.Contains(cardURL, "\\") || strings.ContainsAny(cardURL, " \t\r\n") {
		return "", errors.New("链接格式错误")
	}
	if strings.HasPrefix(cardURL, "/") {
		if strings.HasPrefix(cardURL, "//") {
			return "", errors.New("链接格式错误")
		}
		parsed, err := url.ParseRequestURI(cardURL)
		if err != nil || parsed.Scheme != "" || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
			return "", errors.New("链接格式错误")
		}
		return parsed.String(), nil
	}
	lowerURL := strings.ToLower(cardURL)
	if !strings.HasPrefix(lowerURL, "http://") && !strings.HasPrefix(lowerURL, "https://") {
		return "", errors.New("只支持站内相对路径或 http、https 链接")
	}
	parsed, err := url.Parse(cardURL)
	if err != nil || parsed.Host == "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return "", errors.New("链接格式错误")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("只支持站内相对路径或 http、https 链接")
	}
	return parsed.String(), nil
}

var _ messageapp.BodyProcessor = (*Service)(nil)
