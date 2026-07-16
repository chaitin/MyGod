package messagecontent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	entitycardapp "app/internal/application/entitycard"
	messageapp "app/internal/application/message"
	"app/internal/store"
)

func TestServiceNormalizesAndFinalizesCoreMessageBodies(t *testing.T) {
	service := NewService(Dependencies{
		FetchLinkTitle: func(_ context.Context, linkURL string) (string, error) {
			if linkURL != "https://example.com/path" {
				t.Fatalf("link URL = %q", linkURL)
			}
			return "  Example &amp; Docs  ", nil
		},
	})
	for _, testCase := range []struct {
		name        string
		raw         string
		wantSummary string
	}{
		{"text", `{"type":"text","content":"  hello  "}`, "hello"},
		{"markdown", `{"type":"markdown","content":"| 姓名 | 年龄 |\n| --- | --- |\n| 张三 | 18 |"}`, "姓名 年龄\n张三 18"},
		{"link", `{"type":"link","url":"https://example.com/path"}`, "[链接] Example & Docs"},
		{"card", `{"type":"card","title":" Card ","description":" Desc ","url":"/projects/one"}`, "[卡片] Card"},
		{"chart", `{"type":"chart","chart_type":"line","title":"趋势","description":"描述","data":{"labels":["一","二"],"series":[{"name":"数量","values":[1,2]}]}}`, "[图表] 趋势"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			prepared, err := service.Prepare(context.Background(), "", json.RawMessage(testCase.raw))
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			_, summary, err := service.Finalize(context.Background(), prepared)
			if err != nil {
				t.Fatalf("finalize: %v", err)
			}
			if summary != testCase.wantSummary {
				t.Fatalf("summary = %q, want %q", summary, testCase.wantSummary)
			}
		})
	}
}

func TestServiceResolvesEntityCardsAndMapsErrors(t *testing.T) {
	resolver := &messageContentEntityResolver{card: entitycardapp.Card{
		Title: "任务 - 整理需求", Description: "状态: 待办", URL: "/projects/p?taskId=t",
	}}
	service := NewService(Dependencies{EntityCards: resolver})
	accountID := "10000000-0000-0000-0000-000000000001"
	entityID := "10000000-0000-0000-0000-000000000002"
	prepared, err := service.Prepare(context.Background(), accountID, json.RawMessage(
		`{"type":"entity_card","entity_type":"task","entity_id":"`+entityID+`"}`,
	))
	if err != nil {
		t.Fatalf("prepare entity card: %v", err)
	}
	if resolver.command.AccountID != accountID || resolver.command.EntityID != entityID || resolver.command.EntityType != "task" {
		t.Fatalf("resolve command = %#v", resolver.command)
	}
	var body cardBody
	if err := json.Unmarshal(prepared, &body); err != nil || body.Type != TypeCard || body.Title != resolver.card.Title {
		t.Fatalf("prepared body = %s, decoded = %#v, err = %v", prepared, body, err)
	}

	resolver.err = &entitycardapp.Error{Code: entitycardapp.CodeNotFound, Message: "对象不存在或无权访问"}
	_, err = service.Prepare(context.Background(), accountID, json.RawMessage(
		`{"type":"entity_card","entity_type":"task","entity_id":"`+entityID+`"}`,
	))
	if messageapp.ErrorCodeOf(err) != messageapp.CodeNotFound {
		t.Fatalf("not found error = %v, code = %q", err, messageapp.ErrorCodeOf(err))
	}
}

func TestServiceSanitizesForwardBodiesAndPreservesLimits(t *testing.T) {
	service := NewService(Dependencies{})
	userID := "10000000-0000-0000-0000-000000000001"
	raw := json.RawMessage(`{"type":"markdown","content":"Hi {(@user/` + userID + `)}"}`)
	body, summary, metrics, err := service.SanitizeForwardBody(raw, map[string]string{"user/" + userID: "A*B"}, 0)
	if err != nil {
		t.Fatalf("sanitize markdown: %v", err)
	}
	if string(body) != `{"type":"markdown","content":"Hi @A\\*B"}` || summary != "Hi @A\\*B" || metrics.LeafCount != 1 {
		t.Fatalf("body = %s, summary = %q, metrics = %#v", body, summary, metrics)
	}

	bundle, err := json.Marshal(forwardBundleBody{
		Type: TypeForwardBundle,
		Items: []forwardBundleItem{{
			Body: json.RawMessage(`{"type":"text","content":"hello"}`), SenderName: "Alice",
			SenderType: store.MessageSenderTypeUser, SentAt: time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC),
		}},
	})
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	_, summary, metrics, err = service.SanitizeForwardBody(bundle, nil, 0)
	if err != nil || summary != "[聊天记录] 1 条 - hello" || metrics.BundleDepth != 1 || metrics.LeafCount != 1 {
		t.Fatalf("bundle summary = %q, metrics = %#v, err = %v", summary, metrics, err)
	}
	if _, _, _, err := service.SanitizeForwardBody(json.RawMessage(`{"type":"unknown"}`), nil, 0); !errors.Is(err, messageapp.ErrForwardUnsupportedMessage) {
		t.Fatalf("unsupported error = %v", err)
	}
}

func TestServiceBuildsTaskNotificationAndReminderCards(t *testing.T) {
	service := NewService(Dependencies{})
	body, summary, err := service.BuildTaskNotificationBody(context.Background(), messageapp.TaskNotificationCommand{
		ID: "task-id", ProjectID: "project-id", Title: "整理需求",
	})
	if err != nil || summary != "[卡片] 任务动态 - 整理需求" || !strings.Contains(string(body), `"description":"暂无描述"`) {
		t.Fatalf("notification body = %s, summary = %q, err = %v", body, summary, err)
	}
	occurrence := time.Date(2026, 7, 15, 8, 30, 0, 0, time.UTC)
	body, summary, err = service.BuildTaskReminderBody(context.Background(), messageapp.TaskReminderNotificationCommand{
		ID: "task-id", ProjectID: "project-id", Title: "整理需求", Timezone: "Asia/Shanghai", OccurrenceAt: occurrence,
	})
	if err != nil || summary != "[卡片] 任务提醒 - 整理需求" || !strings.Contains(string(body), "2026 年 7 月 15 日 16:30") {
		t.Fatalf("reminder body = %s, summary = %q, err = %v", body, summary, err)
	}
}

type messageContentEntityResolver struct {
	card    entitycardapp.Card
	err     error
	command entitycardapp.ResolveCommand
}

func (r *messageContentEntityResolver) Resolve(_ context.Context, cmd entitycardapp.ResolveCommand) (entitycardapp.Card, error) {
	r.command = cmd
	return r.card, r.err
}
