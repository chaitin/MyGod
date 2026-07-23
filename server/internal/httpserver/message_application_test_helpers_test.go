package httpserver

import (
	"context"
	"encoding/json"

	messageapp "app/internal/application/message"
)

type fixedApplicationMessageBodyProcessor struct{}

func (fixedApplicationMessageBodyProcessor) Prepare(_ context.Context, _ string, body json.RawMessage) (json.RawMessage, error) {
	return body, nil
}

func (fixedApplicationMessageBodyProcessor) Finalize(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
	return body, "hello", nil
}

type applicationMessageNotificationRecorder struct {
	created func([]messageapp.Delivery)
}

func (r applicationMessageNotificationRecorder) PublishMessageCreated(_ context.Context, deliveries []messageapp.Delivery) {
	if r.created != nil {
		r.created(deliveries)
	}
}

func (r applicationMessageNotificationRecorder) PublishSharedMessageCreated(_ context.Context, userIDs []string, message messageapp.Message) {
	deliveries := make([]messageapp.Delivery, 0, len(userIDs))
	for _, userID := range userIDs {
		deliveries = append(deliveries, messageapp.Delivery{Message: message, UserID: userID})
	}
	r.PublishMessageCreated(context.Background(), deliveries)
}

func (applicationMessageNotificationRecorder) PublishMessageUpdated(context.Context, []messageapp.Delivery) {
}

func (applicationMessageNotificationRecorder) PublishMembersMentioned(context.Context, []string, string, int64) {
}

func (applicationMessageNotificationRecorder) PublishMembersChoiceReceived(context.Context, []string, string, int64) {
}
func (applicationMessageNotificationRecorder) PublishMessageChoiceUpdated(context.Context, []string, messageapp.ChoiceUpdatedEvent) {
}
