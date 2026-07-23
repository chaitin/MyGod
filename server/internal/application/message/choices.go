package message

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"app/internal/application/conversationaccess"
	"app/internal/store"

	"gorm.io/gorm"
)

type choiceDefinitionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type choiceDefinition struct {
	Content     string                   `json:"content"`
	ContentType string                   `json:"content_type"`
	Options     []choiceDefinitionOption `json:"options"`
	Selection   string                   `json:"selection"`
	Type        string                   `json:"type"`
}

func isChoiceMessageBody(body json.RawMessage) bool {
	var envelope struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(body, &envelope) == nil && strings.TrimSpace(envelope.Type) == "choice"
}

func parseChoiceDefinition(body json.RawMessage) (choiceDefinition, error) {
	var value choiceDefinition
	if err := json.Unmarshal(body, &value); err != nil || strings.TrimSpace(value.Type) != "choice" {
		return choiceDefinition{}, errors.New("message is not a choice")
	}
	return value, nil
}

func emptyChoiceState(body json.RawMessage) *ChoiceState {
	definition, err := parseChoiceDefinition(body)
	if err != nil {
		return nil
	}
	options := make([]ChoiceOptionState, len(definition.Options))
	for index, option := range definition.Options {
		options[index] = ChoiceOptionState{ID: option.ID}
	}
	return &ChoiceState{MyOptionIDs: []string{}, Options: options}
}

func attachMessageChoices(db *gorm.DB, messages []Message, currentUserID string) error {
	messageIDs := make([]string, 0, len(messages))
	byID := make(map[string]*Message, len(messages))
	for index := range messages {
		messages[index].Choice = nil
		if messages[index].RevokedAt != nil {
			continue
		}
		state := emptyChoiceState(messages[index].Body)
		if state == nil {
			continue
		}
		messages[index].Choice = state
		messageIDs = append(messageIDs, messages[index].ID)
		byID[messages[index].ID] = &messages[index]
	}
	if len(messageIDs) == 0 {
		return nil
	}
	var responses []store.MessageChoiceResponse
	if err := db.Where("message_id IN ?", messageIDs).Order("created_at ASC, id ASC").Find(&responses).Error; err != nil {
		return err
	}
	for _, response := range responses {
		message := byID[response.MessageID]
		if message == nil || message.Choice == nil {
			continue
		}
		var optionIDs []string
		if err := json.Unmarshal(response.OptionIDs, &optionIDs); err != nil {
			return err
		}
		message.Choice.ResponseCount++
		counts := make(map[string]*ChoiceOptionState, len(message.Choice.Options))
		for index := range message.Choice.Options {
			counts[message.Choice.Options[index].ID] = &message.Choice.Options[index]
		}
		for _, optionID := range optionIDs {
			if option := counts[optionID]; option != nil {
				option.ResponseCount++
			}
		}
		if response.UserID == currentUserID {
			message.Choice.MyOptionIDs = append([]string(nil), optionIDs...)
		}
	}
	return nil
}

func updateConversationChoiceSeq(
	db *gorm.DB,
	access conversationaccess.Context,
	seq int64,
	body json.RawMessage,
	userIDs []string,
	now time.Time,
) ([]string, error) {
	if !isChoiceMessageBody(body) || len(userIDs) == 0 {
		return nil, nil
	}
	if access.IsTopic() {
		result := db.Model(&store.ConversationTopicParticipant{}).
			Where("conversation_id = ? AND participant_type = ? AND participant_id IN ?", access.Conversation.ID, store.ConversationMemberTypeUser, userIDs).
			Updates(map[string]any{
				"last_choice_seq": gorm.Expr("CASE WHEN last_choice_seq > ? THEN last_choice_seq ELSE ? END", seq, seq),
				"updated_at":      now,
			})
		return userIDs, result.Error
	}
	result := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id IN ? AND left_at IS NULL", access.Conversation.ID, store.ConversationMemberTypeUser, userIDs).
		Update("last_choice_seq", gorm.Expr("CASE WHEN last_choice_seq > ? THEN last_choice_seq ELSE ? END", seq, seq))
	return userIDs, result.Error
}

type choiceSeqRecipient struct {
	ID             string
	VisibleFromSeq int64
}

func (s *Service) rewindConversationChoiceSeqOnRevoke(
	db *gorm.DB,
	access conversationaccess.Context,
	message store.Message,
	now time.Time,
) error {
	if !isChoiceMessageBody(message.Body) {
		return nil
	}
	recipients, err := loadChoiceSeqRecipients(db, access, message.Seq)
	if err != nil || len(recipients) == 0 {
		return err
	}

	targets := make(map[string]int64, len(recipients))
	unresolved := make(map[string]int64, len(recipients))
	minimumVisibleSeq := message.Seq
	for _, recipient := range recipients {
		visibleFromSeq := recipient.VisibleFromSeq
		if visibleFromSeq < 1 {
			visibleFromSeq = 1
		}
		targets[recipient.ID] = 0
		unresolved[recipient.ID] = visibleFromSeq
		if visibleFromSeq < minimumVisibleSeq {
			minimumVisibleSeq = visibleFromSeq
		}
	}

	beforeSeq := message.Seq
	ctx := messageStorageContext(db)
	for len(unresolved) > 0 {
		messages, loadErr := s.loadStoredMessagePage(ctx, db, storedMessagePageQuery{
			ConversationID: access.Conversation.ID, VisibleFromSeq: minimumVisibleSeq,
			BeforeSeq: &beforeSeq, Limit: 200, Descending: true,
		})
		if loadErr != nil {
			return loadErr
		}
		if len(messages) == 0 {
			break
		}
		for _, candidate := range messages {
			if candidate.RevokedAt != nil || !isChoiceMessageBody(candidate.Body) {
				continue
			}
			for recipientID, visibleFromSeq := range unresolved {
				if candidate.Seq < visibleFromSeq {
					continue
				}
				targets[recipientID] = candidate.Seq
				delete(unresolved, recipientID)
			}
			if len(unresolved) == 0 {
				break
			}
		}
		oldestSeq := messages[len(messages)-1].Seq
		for recipientID, visibleFromSeq := range unresolved {
			if oldestSeq <= visibleFromSeq {
				delete(unresolved, recipientID)
			}
		}
		if oldestSeq >= beforeSeq {
			break
		}
		beforeSeq = oldestSeq
	}

	return updateChoiceSeqRecipients(db, access, targets, message.Seq, now)
}

func loadChoiceSeqRecipients(db *gorm.DB, access conversationaccess.Context, currentSeq int64) ([]choiceSeqRecipient, error) {
	if access.IsTopic() {
		var participants []store.ConversationTopicParticipant
		if err := db.Select("participant_id", "history_visible_from_seq").Where(
			"conversation_id = ? AND participant_type = ? AND last_choice_seq = ?",
			access.Conversation.ID, store.ConversationMemberTypeUser, currentSeq,
		).Find(&participants).Error; err != nil {
			return nil, err
		}
		result := make([]choiceSeqRecipient, len(participants))
		for index, participant := range participants {
			result[index] = choiceSeqRecipient{ID: participant.ParticipantID, VisibleFromSeq: participant.HistoryVisibleFromSeq}
		}
		return result, nil
	}
	var members []store.ConversationMember
	if err := db.Select("member_id", "history_visible_from_seq").Where(
		"conversation_id = ? AND member_type = ? AND left_at IS NULL AND last_choice_seq = ?",
		access.Conversation.ID, store.ConversationMemberTypeUser, currentSeq,
	).Find(&members).Error; err != nil {
		return nil, err
	}
	result := make([]choiceSeqRecipient, len(members))
	for index, member := range members {
		result[index] = choiceSeqRecipient{ID: member.MemberID, VisibleFromSeq: member.HistoryVisibleFromSeq}
	}
	return result, nil
}

func updateChoiceSeqRecipients(db *gorm.DB, access conversationaccess.Context, targets map[string]int64, expectedSeq int64, now time.Time) error {
	grouped := make(map[int64][]string)
	for recipientID, targetSeq := range targets {
		grouped[targetSeq] = append(grouped[targetSeq], recipientID)
	}
	for targetSeq, recipientIDs := range grouped {
		if access.IsTopic() {
			if err := db.Model(&store.ConversationTopicParticipant{}).Where(
				"conversation_id = ? AND participant_type = ? AND participant_id IN ? AND last_choice_seq = ?",
				access.Conversation.ID, store.ConversationMemberTypeUser, recipientIDs, expectedSeq,
			).Updates(map[string]any{"last_choice_seq": targetSeq, "updated_at": now}).Error; err != nil {
				return err
			}
			continue
		}
		if err := db.Model(&store.ConversationMember{}).Where(
			"conversation_id = ? AND member_type = ? AND member_id IN ? AND left_at IS NULL AND last_choice_seq = ?",
			access.Conversation.ID, store.ConversationMemberTypeUser, recipientIDs, expectedSeq,
		).Update("last_choice_seq", targetSeq).Error; err != nil {
			return err
		}
	}
	return nil
}
