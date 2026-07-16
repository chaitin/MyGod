package messagecontent

import (
	"context"
	"encoding/json"

	entitycardapp "app/internal/application/entitycard"
)

const (
	TypeText          = "text"
	TypeMarkdown      = "markdown"
	TypeLink          = "link"
	TypeCard          = "card"
	TypeChart         = "chart"
	TypeEntityCard    = "entity_card"
	TypeFile          = "file"
	TypeImage         = "image"
	TypeVoice         = "voice"
	TypeForwardBundle = "forward_bundle"
)

type LinkTitleFetcher func(context.Context, string) (string, error)

type Dependencies struct {
	EntityCards    entitycardapp.Resolver
	FetchLinkTitle LinkTitleFetcher
}

type Service struct {
	entityCards    entitycardapp.Resolver
	fetchLinkTitle LinkTitleFetcher
}

func NewService(deps Dependencies) *Service {
	return &Service{entityCards: deps.EntityCards, fetchLinkTitle: deps.FetchLinkTitle}
}

type envelope struct {
	Type string `json:"type"`
}

type textBody struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type markdownBody struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type linkBody struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type cardBody struct {
	Description string `json:"description"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	URL         string `json:"url"`
}

type entityCardBody struct {
	EntityID   string `json:"entity_id"`
	EntityType string `json:"entity_type"`
	Type       string `json:"type"`
}

type fileBody struct {
	Type      string `json:"type"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

type imageBody struct {
	Type   string `json:"type"`
	FileID string `json:"file_id"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type voiceBody struct {
	Type        string `json:"type"`
	FileID      string `json:"file_id"`
	DurationMS  int    `json:"duration_ms"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	Transcript  string `json:"transcript"`
}

type bodyHandler interface {
	Type() string
	Validate(json.RawMessage) error
	Normalize(context.Context, json.RawMessage) (json.RawMessage, error)
	Summary(json.RawMessage) (string, error)
}

type bodyFinalizer interface {
	Finalize(context.Context, json.RawMessage) (json.RawMessage, error)
}
