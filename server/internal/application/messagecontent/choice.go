package messagecontent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"app/internal/messageformat"
)

const (
	choiceContentTypeText     = "text"
	choiceContentTypeMarkdown = "markdown"
	choiceSelectionSingle     = "single"
	choiceSelectionMultiple   = "multiple"
	maxChoiceOptions          = 20
	maxChoiceOptionIDRunes    = 64
	maxChoiceOptionLabelRunes = 200
)

type choiceOptionBody struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type choiceBody struct {
	Content     string             `json:"content"`
	ContentType string             `json:"content_type"`
	Options     []choiceOptionBody `json:"options"`
	Selection   string             `json:"selection"`
	Type        string             `json:"type"`
}

type choiceHandler struct{}

func (choiceHandler) Type() string { return TypeChoice }

func (h choiceHandler) Validate(raw json.RawMessage) error {
	_, err := h.normalize(raw)
	return err
}

func (h choiceHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	body, err := h.normalize(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(body)
}

func (choiceHandler) Summary(raw json.RawMessage) (string, error) {
	var body choiceBody
	if json.Unmarshal(raw, &body) != nil {
		return "", errors.New(messageBodyMalformed)
	}
	content := strings.TrimSpace(body.Content)
	if body.ContentType == choiceContentTypeMarkdown {
		var err error
		content, err = messageformat.MarkdownPlainText(content)
		if err != nil {
			return "", err
		}
	}
	return "[选择] " + content, nil
}

func (h choiceHandler) normalize(raw json.RawMessage) (choiceBody, error) {
	var body choiceBody
	if json.Unmarshal(raw, &body) != nil {
		return choiceBody{}, errors.New(messageBodyMalformed)
	}
	if strings.TrimSpace(body.Type) != h.Type() {
		return choiceBody{}, errors.New("消息类型错误")
	}
	body.ContentType = strings.TrimSpace(body.ContentType)
	if body.ContentType != choiceContentTypeText && body.ContentType != choiceContentTypeMarkdown {
		return choiceBody{}, errors.New("选择内容类型必须是 text 或 markdown")
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" {
		return choiceBody{}, errors.New("选择内容不能为空")
	}
	if utf8.RuneCountInString(body.Content) > maxTextLength {
		return choiceBody{}, errors.New("选择内容不能超过 5000 个字符")
	}
	body.Selection = strings.TrimSpace(body.Selection)
	if body.Selection != choiceSelectionSingle && body.Selection != choiceSelectionMultiple {
		return choiceBody{}, errors.New("选择模式必须是 single 或 multiple")
	}
	if len(body.Options) < 2 || len(body.Options) > maxChoiceOptions {
		return choiceBody{}, errors.New("选择项数量必须在 2 到 20 之间")
	}
	seen := make(map[string]struct{}, len(body.Options))
	for index := range body.Options {
		option := &body.Options[index]
		option.ID = strings.TrimSpace(option.ID)
		option.Label = strings.TrimSpace(option.Label)
		if !validChoiceOptionID(option.ID) {
			return choiceBody{}, errors.New("选择项 ID 格式错误")
		}
		if _, ok := seen[option.ID]; ok {
			return choiceBody{}, errors.New("选择项 ID 不能重复")
		}
		seen[option.ID] = struct{}{}
		if option.Label == "" {
			return choiceBody{}, errors.New("选择项内容不能为空")
		}
		if utf8.RuneCountInString(option.Label) > maxChoiceOptionLabelRunes {
			return choiceBody{}, errors.New("选择项内容不能超过 200 个字符")
		}
		for _, character := range option.Label {
			if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
				return choiceBody{}, errors.New("选择项内容不能包含换行或控制字符")
			}
		}
	}
	body.Type = h.Type()
	return body, nil
}

func validChoiceOptionID(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > maxChoiceOptionIDRunes {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_') {
			return false
		}
	}
	return true
}
