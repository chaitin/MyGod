package messagecontent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	entitycardapp "app/internal/application/entitycard"
	messageapp "app/internal/application/message"
)

func (s *Service) BuildTaskNotificationBody(
	ctx context.Context,
	cmd messageapp.TaskNotificationCommand,
) (json.RawMessage, string, error) {
	description := strings.TrimSpace(cmd.Description)
	if description == "" {
		description = "暂无描述"
	}
	return s.buildTaskCard(ctx, "任务动态", cmd.Title, truncateCardDescription(description), cmd.ProjectID, cmd.ID)
}

func (s *Service) BuildTaskReminderBody(
	ctx context.Context,
	cmd messageapp.TaskReminderNotificationCommand,
) (json.RawMessage, string, error) {
	location, err := time.LoadLocation(cmd.Timezone)
	if err != nil {
		location = time.UTC
	}
	lines := []string{"提醒时间：" + cmd.OccurrenceAt.In(location).Format("2006 年 1 月 2 日 15:04")}
	if description := strings.TrimSpace(cmd.Description); description != "" {
		lines = append(lines, description)
	}
	return s.buildTaskCard(
		ctx, "任务提醒", cmd.Title, truncateCardDescription(strings.Join(lines, "\n")), cmd.ProjectID, cmd.ID,
	)
}

func (s *Service) buildTaskCard(
	ctx context.Context,
	label, title, description, projectID, taskID string,
) (json.RawMessage, string, error) {
	body, err := json.Marshal(cardBody{
		Description: description, Title: entitycardapp.Title(label, title), Type: TypeCard,
		URL: fmt.Sprintf("/projects/%s?taskId=%s", projectID, taskID),
	})
	if err != nil {
		return nil, "", err
	}
	normalized, err := (cardHandler{}).Normalize(ctx, body)
	if err != nil {
		return nil, "", err
	}
	summary, err := (cardHandler{}).Summary(normalized)
	if err != nil {
		return nil, "", err
	}
	return normalized, summary, nil
}

func truncateCardDescription(description string) string {
	characters := []rune(description)
	if len(characters) <= maxCardDescription {
		return description
	}
	if maxCardDescription <= 1 {
		return string(characters[:maxCardDescription])
	}
	return strings.TrimSpace(string(characters[:maxCardDescription-1])) + "…"
}

var _ messageapp.TaskNotificationBodyBuilder = (*Service)(nil)
var _ messageapp.TaskReminderBodyBuilder = (*Service)(nil)
