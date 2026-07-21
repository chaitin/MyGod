package message

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"app/internal/store"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxReactionTextBytes           = 128
	maxReactionTextRunes           = 32
	maxReactionsPerUserMessage     = 8
	maxDistinctReactionsPerMessage = 32
	maxReactionSummaryUsers        = 10
)

var (
	errReactionMessageUnavailable = errors.New("reaction message unavailable")
	errReactionTopicParticipation = errors.New("reaction topic participation required")
	errReactionUserLimit          = errors.New("reaction user limit reached")
	errReactionMessageLimit       = errors.New("reaction message limit reached")
)

type reactionMessageReference struct {
	ConversationID string
	ID             string
	RevokedAt      *time.Time
	SenderType     string
	Seq            int64
}

func (s *Service) SetReaction(ctx context.Context, cmd SetReactionCommand) (SetReactionResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return SetReactionResult{}, InvalidRequestError(err.Error(), err)
	}
	messageID, err := normalizeRequiredUUID(cmd.MessageID, "消息 ID")
	if err != nil {
		return SetReactionResult{}, InvalidRequestError(err.Error(), err)
	}
	accountID, err := normalizeRequiredUUID(cmd.AccountID, "用户 ID")
	if err != nil {
		return SetReactionResult{}, InvalidRequestError(err.Error(), err)
	}
	reactionText, err := normalizeReactionText(cmd.Text)
	if err != nil {
		return SetReactionResult{}, InvalidRequestError(err.Error(), err)
	}

	result := SetReactionResult{ConversationID: conversationID, MessageID: messageID}
	recipients := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access, _, err := validateSetReactionAccess(tx, conversationID, messageID, accountID, cmd.Reacted)
		if err != nil {
			return err
		}
		if err := lockReactionAccessRows(tx, access); err != nil {
			return err
		}

		now := time.Now().UTC()
		state, err := lockMessageReactionState(tx, messageID, now)
		if err != nil {
			return err
		}
		access, _, err = validateSetReactionAccess(tx, conversationID, messageID, accountID, cmd.Reacted)
		if err != nil {
			return err
		}

		var existing store.MessageReaction
		existingQuery := tx.Where("message_id = ? AND user_id = ? AND text = ?", messageID, accountID, reactionText).
			Limit(1).Find(&existing)
		if existingQuery.Error != nil {
			return existingQuery.Error
		}
		exists := existingQuery.RowsAffected > 0
		if cmd.Reacted && !exists {
			var userReactionCount int64
			if err := tx.Model(&store.MessageReaction{}).
				Where("message_id = ? AND user_id = ?", messageID, accountID).
				Count(&userReactionCount).Error; err != nil {
				return err
			}
			if userReactionCount >= maxReactionsPerUserMessage {
				return errReactionUserLimit
			}
			var sameTextCount int64
			if err := tx.Model(&store.MessageReaction{}).
				Where("message_id = ? AND text = ?", messageID, reactionText).
				Count(&sameTextCount).Error; err != nil {
				return err
			}
			if sameTextCount == 0 {
				var distinctCount int64
				if err := tx.Model(&store.MessageReaction{}).
					Where("message_id = ?", messageID).
					Distinct("text").Count(&distinctCount).Error; err != nil {
					return err
				}
				if distinctCount >= maxDistinctReactionsPerMessage {
					return errReactionMessageLimit
				}
			}
			if err := tx.Create(&store.MessageReaction{
				MessageID: messageID, UserID: accountID, Text: reactionText, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
			result.Changed = true
		}
		if !cmd.Reacted && exists {
			if err := tx.Where("message_id = ? AND user_id = ? AND text = ?", messageID, accountID, reactionText).
				Delete(&store.MessageReaction{}).Error; err != nil {
				return err
			}
			result.Changed = true
		}
		if result.Changed {
			if err := incrementMessageReactionVersion(tx, &state, now); err != nil {
				return err
			}
			recipients, err = loadConversationDeliveryUserIDs(tx, access.Context)
			if err != nil {
				return err
			}
		}
		result.ReactionVersion = state.Version
		summaries, err := loadReactionSummaries(tx, []string{messageID}, accountID)
		result.Reactions = summaries[messageID]
		return err
	})
	if err != nil {
		return SetReactionResult{}, mapReactionError(err)
	}

	if result.Changed && s.reactionNotifications != nil {
		counts := make([]ReactionCount, len(result.Reactions))
		for index, reaction := range result.Reactions {
			counts[index] = ReactionCount{
				Count: reaction.Count, Text: reaction.Text, Users: reaction.Users,
			}
		}
		s.reactionNotifications.PublishMessageReactionsUpdated(ctx, recipients, ReactionEvent{
			ActorReacted: cmd.Reacted, ActorText: reactionText, ActorUserID: accountID,
			ConversationID: conversationID, MessageID: messageID,
			ReactionVersion: result.ReactionVersion, Reactions: counts,
		})
	}
	return result, nil
}

// lockReactionAccessRows prevents membership, conversation and topic state
// from changing while a reaction is written. SHARE locks remain compatible
// with other reactions, so messages in the same conversation do not serialize.
func lockReactionAccessRows(db *gorm.DB, access userConversationAccess) error {
	lock := clause.Locking{Strength: "SHARE"}
	if access.Context.ParentConversation != nil {
		var parent store.Conversation
		if err := db.Clauses(lock).First(&parent, "id = ?", access.Context.ParentConversation.ID).Error; err != nil {
			return err
		}
	}
	var conversation store.Conversation
	if err := db.Clauses(lock).First(&conversation, "id = ?", access.Context.Conversation.ID).Error; err != nil {
		return err
	}
	if access.Context.Topic != nil {
		var topic store.ConversationTopic
		if err := db.Clauses(lock).First(&topic, "conversation_id = ?", access.Context.Conversation.ID).Error; err != nil {
			return err
		}
	}
	var member store.ConversationMember
	if err := db.Clauses(lock).First(
		&member,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		access.Member.ConversationID, access.Member.MemberType, access.Member.MemberID,
	).Error; err != nil {
		return err
	}
	if access.Participant != nil {
		var participant store.ConversationTopicParticipant
		if err := db.Clauses(lock).First(
			&participant,
			"conversation_id = ? AND participant_type = ? AND participant_id = ?",
			access.Participant.ConversationID, access.Participant.ParticipantType, access.Participant.ParticipantID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateSetReactionAccess(db *gorm.DB, conversationID, messageID, accountID string, reacted bool) (userConversationAccess, reactionMessageReference, error) {
	access, err := loadUserConversationAccess(db, conversationID, accountID, false)
	if err != nil {
		return userConversationAccess{}, reactionMessageReference{}, err
	}
	message, err := loadReactionMessageReference(db, conversationID, messageID)
	if err != nil {
		return userConversationAccess{}, reactionMessageReference{}, err
	}
	if message.Seq < access.visibleFromSeq() {
		return userConversationAccess{}, reactionMessageReference{}, errConversationAccessDenied
	}
	// Removing a reaction is still a write. Keep retained app-direct history
	// readable after access is revoked, but reject every mutation in it.
	if err := validateUserDirectAppAccess(db, access); err != nil {
		return userConversationAccess{}, reactionMessageReference{}, err
	}
	if reacted {
		if message.RevokedAt != nil || message.SenderType == store.MessageSenderTypeSystem {
			return userConversationAccess{}, reactionMessageReference{}, errReactionMessageUnavailable
		}
		if err := validateUserConversationSendable(db, access); err != nil {
			return userConversationAccess{}, reactionMessageReference{}, err
		}
		if access.Context.IsTopic() && access.Participant == nil {
			return userConversationAccess{}, reactionMessageReference{}, errReactionTopicParticipation
		}
	}
	return access, message, nil
}

func (s *Service) ListReactionSnapshots(ctx context.Context, cmd ListReactionSnapshotsCommand) (ListReactionSnapshotsResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return ListReactionSnapshotsResult{}, InvalidRequestError(err.Error(), err)
	}
	accountID, err := normalizeRequiredUUID(cmd.AccountID, "用户 ID")
	if err != nil {
		return ListReactionSnapshotsResult{}, InvalidRequestError(err.Error(), err)
	}
	if len(cmd.MessageIDs) == 0 {
		return ListReactionSnapshotsResult{}, InvalidRequestError("消息 ID 不能为空", nil)
	}
	if len(cmd.MessageIDs) > MaxReactionSnapshotIDs {
		return ListReactionSnapshotsResult{}, InvalidRequestError("一次最多查询 100 条消息", nil)
	}
	messageIDs := make([]string, 0, len(cmd.MessageIDs))
	seen := make(map[string]struct{}, len(cmd.MessageIDs))
	for _, rawID := range cmd.MessageIDs {
		messageID, normalizeErr := normalizeRequiredUUID(rawID, "消息 ID")
		if normalizeErr != nil {
			return ListReactionSnapshotsResult{}, InvalidRequestError(normalizeErr.Error(), normalizeErr)
		}
		if _, exists := seen[messageID]; exists {
			continue
		}
		seen[messageID] = struct{}{}
		messageIDs = append(messageIDs, messageID)
	}

	result := ListReactionSnapshotsResult{ConversationID: conversationID}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access, err := loadUserConversationAccess(tx, conversationID, accountID, false)
		if err != nil {
			return err
		}
		references, err := loadReactionMessageReferences(tx, conversationID, messageIDs)
		if err != nil {
			return err
		}
		messages := make([]Message, len(references))
		for index, reference := range references {
			if reference.Seq < access.visibleFromSeq() {
				return errConversationAccessDenied
			}
			messages[index] = Message{
				ConversationID: reference.ConversationID, ID: reference.ID,
				RevokedAt: reference.RevokedAt, Sender: Identity{Type: reference.SenderType},
			}
		}
		if err := attachMessageReactions(tx, messages, accountID); err != nil {
			return err
		}
		result.Snapshots = make([]ReactionSnapshot, len(messages))
		for index, message := range messages {
			result.Snapshots[index] = ReactionSnapshot{
				MessageID: message.ID, ReactionVersion: message.ReactionVersion, Reactions: message.Reactions,
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return ListReactionSnapshotsResult{}, mapReactionError(err)
	}
	return result, nil
}

func lockMessageReactionState(db *gorm.DB, messageID string, now time.Time) (store.MessageReactionState, error) {
	state := store.MessageReactionState{MessageID: messageID, UpdatedAt: now}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return store.MessageReactionState{}, err
	}
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "message_id = ?", messageID).Error; err != nil {
		return store.MessageReactionState{}, err
	}
	return state, nil
}

func incrementMessageReactionVersion(db *gorm.DB, state *store.MessageReactionState, now time.Time) error {
	if err := db.Model(&store.MessageReactionState{}).Where("message_id = ?", state.MessageID).
		Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": now}).Error; err != nil {
		return err
	}
	return db.First(state, "message_id = ?", state.MessageID).Error
}

type reactionSummaryRecord struct {
	Count     int64
	MessageID string
	Text      string
}

type reactionUserRecord struct {
	MessageID    string
	ReactionRank int64
	Text         string
	UserID       string
	UserName     string
}

func attachMessageReactions(db *gorm.DB, messages []Message, currentUserID string) error {
	if len(messages) == 0 {
		return nil
	}
	messageIDs := make([]string, 0, len(messages))
	reactionMessageIDs := make([]string, 0, len(messages))
	for index := range messages {
		messages[index].Reactions = []ReactionSummary{}
		messageIDs = append(messageIDs, messages[index].ID)
		if messages[index].RevokedAt == nil && messages[index].Sender.Type != store.MessageSenderTypeSystem {
			reactionMessageIDs = append(reactionMessageIDs, messages[index].ID)
		}
	}
	summaries, err := loadReactionSummaries(db, reactionMessageIDs, currentUserID)
	if err != nil {
		return err
	}
	var states []store.MessageReactionState
	if err := db.Where("message_id IN ?", messageIDs).Find(&states).Error; err != nil {
		return err
	}
	versions := make(map[string]int64, len(states))
	for _, state := range states {
		versions[state.MessageID] = state.Version
	}
	for index := range messages {
		messages[index].ReactionVersion = versions[messages[index].ID]
		if values, ok := summaries[messages[index].ID]; ok {
			messages[index].Reactions = values
		}
	}
	return nil
}

func loadReactionSummaries(db *gorm.DB, messageIDs []string, currentUserID string) (map[string][]ReactionSummary, error) {
	result := make(map[string][]ReactionSummary, len(messageIDs))
	if len(messageIDs) == 0 {
		return result, nil
	}
	var records []reactionSummaryRecord
	if err := db.Model(&store.MessageReaction{}).
		Select("message_id, text, COUNT(*) AS count").
		Where("message_id IN ?", messageIDs).
		Group("message_id, text").
		Order("MIN(created_at) ASC, text ASC").
		Scan(&records).Error; err != nil {
		return nil, err
	}
	reactedByMe := make(map[string]map[string]struct{})
	if currentUserID != "" {
		var own []store.MessageReaction
		if err := db.Select("message_id", "text").
			Where("message_id IN ? AND user_id = ?", messageIDs, currentUserID).
			Find(&own).Error; err != nil {
			return nil, err
		}
		for _, reaction := range own {
			if reactedByMe[reaction.MessageID] == nil {
				reactedByMe[reaction.MessageID] = make(map[string]struct{})
			}
			reactedByMe[reaction.MessageID][reaction.Text] = struct{}{}
		}
	}
	users, err := loadReactionSummaryUsers(db, messageIDs)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		_, own := reactedByMe[record.MessageID][record.Text]
		result[record.MessageID] = append(result[record.MessageID], ReactionSummary{
			Count: record.Count, ReactedByMe: own, Text: record.Text,
			Users: users[record.MessageID][record.Text],
		})
	}
	return result, nil
}

func loadReactionSummaryUsers(db *gorm.DB, messageIDs []string) (map[string]map[string][]ReactionUser, error) {
	result := make(map[string]map[string][]ReactionUser, len(messageIDs))
	if len(messageIDs) == 0 {
		return result, nil
	}
	ranked := db.Table("message_reactions AS mr").
		Select(`mr.message_id, mr.text, mr.user_id,
			COALESCE(NULLIF(TRIM(users.nickname), ''), NULLIF(TRIM(users.name), ''), '用户') AS user_name,
			ROW_NUMBER() OVER (PARTITION BY mr.message_id, mr.text ORDER BY mr.created_at ASC, mr.user_id ASC) AS reaction_rank`).
		Joins("JOIN users ON users.id = mr.user_id").
		Where("mr.message_id IN ?", messageIDs)
	var records []reactionUserRecord
	if err := db.Table("(?) AS ranked_reactions", ranked).
		Select("message_id", "text", "user_id", "user_name", "reaction_rank").
		Where("reaction_rank <= ?", maxReactionSummaryUsers).
		Order("message_id ASC, text ASC, reaction_rank ASC").
		Scan(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		if result[record.MessageID] == nil {
			result[record.MessageID] = make(map[string][]ReactionUser)
		}
		result[record.MessageID][record.Text] = append(
			result[record.MessageID][record.Text], ReactionUser{ID: record.UserID, Name: record.UserName},
		)
	}
	return result, nil
}

func (s *Service) ListReactionUsers(ctx context.Context, cmd ListReactionUsersCommand) (ListReactionUsersResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return ListReactionUsersResult{}, InvalidRequestError(err.Error(), err)
	}
	messageID, err := normalizeRequiredUUID(cmd.MessageID, "消息 ID")
	if err != nil {
		return ListReactionUsersResult{}, InvalidRequestError(err.Error(), err)
	}
	accountID, err := normalizeRequiredUUID(cmd.AccountID, "用户 ID")
	if err != nil {
		return ListReactionUsersResult{}, InvalidRequestError(err.Error(), err)
	}
	reactionText, err := normalizeReactionText(cmd.Text)
	if err != nil {
		return ListReactionUsersResult{}, InvalidRequestError(err.Error(), err)
	}

	db := s.db.WithContext(ctx)
	access, err := loadUserConversationAccess(db, conversationID, accountID, false)
	if err != nil {
		return ListReactionUsersResult{}, mapReactionError(err)
	}
	message, err := loadReactionMessageReference(db, conversationID, messageID)
	if err != nil {
		return ListReactionUsersResult{}, mapReactionError(err)
	}
	if message.Seq < access.visibleFromSeq() {
		return ListReactionUsersResult{}, mapReactionError(errConversationAccessDenied)
	}
	var records []reactionUserRecord
	if err := db.Table("message_reactions AS mr").
		Select(`mr.user_id,
			COALESCE(NULLIF(TRIM(users.nickname), ''), NULLIF(TRIM(users.name), ''), '用户') AS user_name`).
		Joins("JOIN users ON users.id = mr.user_id").
		Where("mr.message_id = ? AND mr.text = ?", messageID, reactionText).
		Order("mr.created_at ASC, mr.user_id ASC").
		Scan(&records).Error; err != nil {
		return ListReactionUsersResult{}, internalError(err)
	}
	users := make([]ReactionUser, len(records))
	for index, record := range records {
		users[index] = ReactionUser{ID: record.UserID, Name: record.UserName}
	}
	return ListReactionUsersResult{
		ConversationID: conversationID, MessageID: messageID, Text: reactionText, Users: users,
	}, nil
}

func loadReactionMessageReference(db *gorm.DB, conversationID, messageID string) (reactionMessageReference, error) {
	if store.MessagePartitioningEnabled(db) {
		var value store.MessageRegistry
		if err := applyOnlineStoredMessageWindow(db).
			Where("id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).
			First(&value).Error; err != nil {
			return reactionMessageReference{}, err
		}
		return reactionMessageReference{
			ConversationID: value.ConversationID, ID: value.ID, RevokedAt: value.RevokedAt,
			SenderType: value.SenderType, Seq: value.Seq,
		}, nil
	}
	var value store.Message
	if err := applyOnlineStoredMessageWindow(db).
		Where("id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).
		First(&value).Error; err != nil {
		return reactionMessageReference{}, err
	}
	return reactionMessageReference{
		ConversationID: value.ConversationID, ID: value.ID, RevokedAt: value.RevokedAt,
		SenderType: value.SenderType, Seq: value.Seq,
	}, nil
}

func loadReactionMessageReferences(db *gorm.DB, conversationID string, messageIDs []string) ([]reactionMessageReference, error) {
	model := any(&store.Message{})
	if store.MessagePartitioningEnabled(db) {
		model = &store.MessageRegistry{}
	}
	var values []reactionMessageReference
	if err := applyOnlineStoredMessageWindow(db.Model(model)).
		Select("conversation_id", "id", "revoked_at", "sender_type", "seq").
		Where("conversation_id = ? AND id IN ? AND deleted_at IS NULL", conversationID, messageIDs).
		Find(&values).Error; err != nil {
		return nil, err
	}
	if len(values) != len(messageIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	byID := make(map[string]reactionMessageReference, len(values))
	for _, value := range values {
		byID[value.ID] = value
	}
	ordered := make([]reactionMessageReference, len(messageIDs))
	for index, messageID := range messageIDs {
		ordered[index] = byID[messageID]
	}
	return ordered, nil
}

func normalizeReactionText(raw string) (string, error) {
	value := norm.NFC.String(strings.TrimSpace(raw))
	if value == "" {
		return "", errors.New("表情内容不能为空")
	}
	if len(value) > maxReactionTextBytes || utf8.RuneCountInString(value) > maxReactionTextRunes {
		return "", errors.New("表情内容不能超过 32 个字符")
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return "", errors.New("表情内容不能包含换行或控制字符")
		}
	}
	return value, nil
}

func mapReactionError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFoundError("消息不存在", err)
	case errors.Is(err, errConversationAccessDenied), errors.Is(err, errReactionTopicParticipation):
		return forbidden("无权操作该消息的表情", err)
	case errors.Is(err, errConversationNotSendable):
		return forbidden("当前会话不能添加表情", err)
	case errors.Is(err, errAppDirectAccessDenied):
		return forbidden("你当前无权直接使用此应用", err)
	case errors.Is(err, errReactionMessageUnavailable):
		return conflict("当前消息不能添加表情", err)
	case errors.Is(err, errReactionUserLimit):
		return conflict("每条消息最多添加 8 个表情", err)
	case errors.Is(err, errReactionMessageLimit):
		return conflict("该消息的表情种类已达上限", err)
	default:
		return internalError(err)
	}
}
