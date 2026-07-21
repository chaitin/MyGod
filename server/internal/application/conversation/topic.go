package conversation

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"app/internal/application/conversationaccess"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxTopicNameLength = 120

var topicMentionTokenPattern = regexp.MustCompile(`\{\(@(?:(user)/(all)|(user|app)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))\)\}`)

type topicCreator struct {
	id         string
	memberType string
	user       store.User
}

func (s *Service) CreateTopic(ctx context.Context, cmd CreateTopicCommand) (CreateTopicResult, error) {
	parentID, err := normalizeConversationID(cmd.ParentConversationID)
	if err != nil {
		return CreateTopicResult{}, invalidRequest(err.Error(), err)
	}
	sourceMessageID, err := normalizeUUID(cmd.SourceMessageID, "来源消息 ID 格式错误")
	if err != nil {
		return CreateTopicResult{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	conversation, created, userIDs, err := s.createTopic(s.db.WithContext(ctx), topicCreator{
		id: actor.ID, memberType: store.ConversationMemberTypeUser, user: actor,
	}, parentID, sourceMessageID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return CreateTopicResult{}, notFound("父会话或来源消息不存在", err)
		case errors.Is(err, ErrAccessDenied):
			return CreateTopicResult{}, forbidden("无权创建话题", err)
		case errors.Is(err, ErrTopicInvalidSource):
			return CreateTopicResult{}, invalidRequest("当前消息不能创建话题", err)
		case errors.Is(err, ErrTopicNested):
			return CreateTopicResult{}, invalidRequest("话题内不能继续创建话题", err)
		default:
			return CreateTopicResult{}, internalError(err)
		}
	}
	item, err := s.loadItem(s.db.WithContext(ctx), conversation, actor.ID)
	if err != nil {
		return CreateTopicResult{}, internalError(err)
	}
	if created && s.notifications != nil {
		s.notifications.PublishTopicEvent(ctx, userIDs, TopicEvent{
			ConversationID: conversation.ID, ParentConversationID: parentID,
			SourceMessageID: sourceMessageID, Type: "created",
		})
	}
	return CreateTopicResult{Conversation: item, Created: created}, nil
}

func (s *Service) CreateTopicAsApp(ctx context.Context, cmd AppCreateTopicCommand) (AppTopicResult, error) {
	appID, err := normalizeUUID(cmd.AppID, "应用 ID 格式错误")
	if err != nil {
		return AppTopicResult{}, invalidRequest(err.Error(), err)
	}
	parentID, err := normalizeConversationID(cmd.ParentConversationID)
	if err != nil {
		return AppTopicResult{}, invalidRequest(err.Error(), err)
	}
	sourceMessageID, err := normalizeUUID(cmd.SourceMessageID, "来源消息 ID 格式错误")
	if err != nil {
		return AppTopicResult{}, invalidRequest(err.Error(), err)
	}
	conversation, created, userIDs, err := s.createTopic(s.db.WithContext(ctx), topicCreator{
		id: appID, memberType: store.ConversationMemberTypeApp,
	}, parentID, sourceMessageID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return AppTopicResult{}, notFound("父会话、来源消息或应用不存在", err)
		case errors.Is(err, ErrAccessDenied):
			return AppTopicResult{}, forbidden("应用无权创建话题", err)
		case errors.Is(err, ErrTopicInvalidSource):
			return AppTopicResult{}, invalidRequest("当前消息不能创建话题", err)
		case errors.Is(err, ErrTopicNested):
			return AppTopicResult{}, invalidRequest("话题内不能继续创建话题", err)
		default:
			return AppTopicResult{}, internalError(err)
		}
	}
	var topic store.ConversationTopic
	if err := s.db.WithContext(ctx).First(&topic, "conversation_id = ?", conversation.ID).Error; err != nil {
		return AppTopicResult{}, internalError(err)
	}
	if created && s.notifications != nil {
		s.notifications.PublishTopicEvent(ctx, userIDs, TopicEvent{
			ConversationID: conversation.ID, ParentConversationID: parentID,
			SourceMessageID: sourceMessageID, Type: "created",
		})
	}
	return newAppTopicResult(conversation, topic, created), nil
}

func (s *Service) createTopic(db *gorm.DB, creator topicCreator, parentID, sourceMessageID string) (store.Conversation, bool, []string, error) {
	var conversation store.Conversation
	created := false
	userIDs := []string{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing store.ConversationTopic
		existingResult := tx.Where("source_message_id = ?", sourceMessageID).Limit(1).Find(&existing)
		if existingResult.Error != nil {
			return existingResult.Error
		}
		if existingResult.RowsAffected > 0 {
			if existing.ParentConversationID != parentID {
				return ErrTopicInvalidSource
			}
			topicAccess, err := conversationaccess.Load(tx, existing.ConversationID, false)
			if err != nil {
				return err
			}
			if _, err := requireActiveTopicActorMember(tx, topicAccess, creator.memberType, creator.id); err != nil {
				return err
			}
			if err := requireTopicCreatorDirectAppAccess(tx, topicAccess, creator); err != nil {
				return err
			}
			if !topicAccess.IsArchived() {
				now := s.now().UTC()
				if err := conversationaccess.EnsureTopicParticipant(
					tx, topicAccess, creator.memberType, creator.id,
					store.TopicParticipantReasonCreator, 0, 0, now,
				); err != nil {
					return err
				}
			}
			return tx.First(&conversation, "id = ?", existing.ConversationID).Error
		}

		parentAccess, err := conversationaccess.Load(tx, parentID, true)
		if err != nil {
			return err
		}
		parent := parentAccess.Conversation
		if parent.Kind == store.ConversationKindTopic {
			return ErrTopicNested
		}
		if parent.Status != store.ConversationStatusActive || parent.PostingPolicy != store.ConversationPostingPolicyOpen {
			return ErrAccessDenied
		}
		member, err := requireTopicCreatorMember(tx, parentAccess, creator)
		if err != nil {
			return ErrAccessDenied
		}
		if err := requireTopicCreatorDirectAppAccess(tx, parentAccess, creator); err != nil {
			return err
		}
		source, err := loadTopicSourceMessage(tx, parent.ID, sourceMessageID)
		if err != nil {
			return err
		}
		if source.Seq < member.HistoryVisibleFromSeq || source.RevokedAt != nil || source.DeletedAt != nil || source.SenderID == nil || (source.SenderType != store.MessageSenderTypeUser && source.SenderType != store.MessageSenderTypeApp) {
			return ErrTopicInvalidSource
		}
		senderName, err := loadTopicSourceSenderName(tx, source)
		if err != nil {
			return err
		}
		topicName, err := resolveTopicName(tx, source.Summary)
		if err != nil {
			return err
		}
		members, err := conversationaccess.ActiveMembers(tx, parentAccess)
		if err != nil {
			return err
		}
		legacyCreatorUserID, err := resolveTopicLegacyCreatorUserID(creator, source, members)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		conversation = store.Conversation{
			ID: uuid.NewString(), Kind: store.ConversationKindTopic, Name: topicName,
			Avatar: parent.Avatar, CreatedByUserID: legacyCreatorUserID, Status: store.ConversationStatusActive,
			PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: parent.Visibility,
			CreatedAt: now, UpdatedAt: now,
		}
		if creator.memberType == store.ConversationMemberTypeApp {
			conversation.CreatedByAppID = &creator.id
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		topic := store.ConversationTopic{
			ConversationID: conversation.ID, ParentConversationID: parent.ID,
			SourceMessageID: source.ID, SourceMessageSeq: source.Seq, SourceMessageBody: source.Body,
			SourceMessageSummary: source.Summary, SourceSenderType: source.SenderType,
			SourceSenderID: source.SenderID, SourceSenderName: senderName,
			SourceMessageCreatedAt: source.CreatedAt, CreatedByUserID: legacyCreatorUserID,
			CreatedAt: now, UpdatedAt: now,
		}
		if creator.memberType == store.ConversationMemberTypeApp {
			topic.CreatedByAppID = &creator.id
		}
		if err := tx.Create(&topic).Error; err != nil {
			return err
		}
		participants := initialTopicParticipants(parent, source, members, creator, conversation.ID, now)
		for _, current := range members {
			if !conversationaccess.SourceMessageVisibleToMember(source.Seq, current) {
				continue
			}
			if current.MemberType == store.ConversationMemberTypeUser {
				userIDs = append(userIDs, current.MemberID)
			}
		}
		if len(participants) > 0 {
			if err := tx.Create(&participants).Error; err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	if err != nil && isUniqueConstraintError(err) {
		var topic store.ConversationTopic
		if findErr := db.First(&topic, "source_message_id = ?", sourceMessageID).Error; findErr == nil {
			if topic.ParentConversationID != parentID {
				return store.Conversation{}, false, nil, ErrTopicInvalidSource
			}
			access, loadErr := conversationaccess.Load(db, topic.ConversationID, false)
			if loadErr == nil {
				if _, accessErr := requireActiveTopicActorMember(db, access, creator.memberType, creator.id); accessErr != nil {
					return store.Conversation{}, false, nil, accessErr
				}
				if accessErr := requireTopicCreatorDirectAppAccess(db, access, creator); accessErr != nil {
					return store.Conversation{}, false, nil, accessErr
				}
				if !access.IsArchived() {
					now := s.now().UTC()
					if participantErr := conversationaccess.EnsureTopicParticipant(
						db, access, creator.memberType, creator.id,
						store.TopicParticipantReasonCreator, 0, 0, now,
					); participantErr != nil {
						return store.Conversation{}, false, nil, participantErr
					}
				}
				return access.Conversation, false, userIDs, nil
			}
		}
	}
	return conversation, created, userIDs, err
}

func (s *Service) GetTopic(ctx context.Context, cmd GetTopicCommand) (TopicDetail, error) {
	topicID, err := normalizeConversationID(cmd.TopicConversationID)
	if err != nil {
		return TopicDetail{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	db := s.db.WithContext(ctx)
	access, err := conversationaccess.Load(db, topicID, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TopicDetail{}, notFound("话题不存在", err)
	}
	if err != nil || !access.IsTopic() {
		if err == nil {
			err = ErrTopicInvalidSource
		}
		return TopicDetail{}, invalidRequest("会话不是话题", err)
	}
	member, err := requireActiveTopicUserMember(db, access, actor.ID)
	if errors.Is(err, ErrAccessDenied) {
		return TopicDetail{}, forbidden("无权访问话题", ErrAccessDenied)
	}
	if err != nil {
		return TopicDetail{}, internalError(err)
	}
	item, err := s.loadItem(db, access.Conversation, actor.ID)
	if err != nil {
		return TopicDetail{}, internalError(err)
	}
	if s.projects != nil {
		values, err := s.projects.ListForConversations(ctx, []string{access.MembershipConversationID})
		if err != nil {
			return TopicDetail{}, internalError(err)
		}
		projects := make([]Project, 0, len(values[access.MembershipConversationID]))
		for _, project := range values[access.MembershipConversationID] {
			projects = append(projects, Project{
				Avatar: project.Avatar, Description: project.Description, ID: project.ID, Name: project.Name,
			})
		}
		item.Projects = &projects
	}
	isUserCreator := access.Topic.CreatedByAppID == nil && access.Topic.CreatedByUserID == actor.ID
	isSourceSender := access.Topic.SourceSenderType == store.MessageSenderTypeUser &&
		access.Topic.SourceSenderID != nil && *access.Topic.SourceSenderID == actor.ID
	canArchive := isUserCreator || isSourceSender || member.Role == store.ConversationMemberRoleOwner || member.Role == store.ConversationMemberRoleAdmin
	var revokedAt *time.Time
	if store.MessagePartitioningEnabled(s.db) {
		var registry store.MessageRegistry
		if result := db.Select("revoked_at").Where("id = ?", access.Topic.SourceMessageID).Limit(1).Find(&registry); result.Error == nil && result.RowsAffected > 0 {
			revokedAt = registry.RevokedAt
		}
	} else {
		var message store.Message
		if result := db.Select("revoked_at").Where("id = ?", access.Topic.SourceMessageID).Limit(1).Find(&message); result.Error == nil && result.RowsAffected > 0 {
			revokedAt = message.RevokedAt
		}
	}
	sourceBody := access.Topic.SourceMessageBody
	if revokedAt != nil {
		sourceBody = nil
	}
	senderAvatar, err := loadTopicSourceSenderAvatar(db, access.Topic.SourceSenderType, access.Topic.SourceSenderID)
	if err != nil {
		return TopicDetail{}, internalError(err)
	}
	return TopicDetail{
		CanArchive:     item.CanSend && canArchive && !access.IsArchived(),
		CanParticipate: item.CanSend && !access.IsArchived() && (item.Topic == nil || !item.Topic.Participating),
		Conversation:   item,
		ParentConversation: Reference{
			ID: access.ParentConversation.ID, Name: item.Topic.ParentConversationName,
			Type: item.Topic.ParentConversationType,
		},
		SourceMessage: TopicSourceMessage{
			Body: sourceBody, CreatedAt: access.Topic.SourceMessageCreatedAt, ID: access.Topic.SourceMessageID,
			RevokedAt: revokedAt, Sender: MessageIdentity{Avatar: senderAvatar, ID: dereferenceString(access.Topic.SourceSenderID), Name: access.Topic.SourceSenderName, Type: access.Topic.SourceSenderType},
			Seq: access.Topic.SourceMessageSeq, Summary: access.Topic.SourceMessageSummary,
		},
	}, nil
}

func (s *Service) ParticipateTopic(ctx context.Context, cmd ParticipateTopicCommand) (Item, error) {
	topicID, err := normalizeConversationID(cmd.TopicConversationID)
	if err != nil {
		return Item{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	var conversation store.Conversation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access, err := conversationaccess.Load(tx, topicID, true)
		if err != nil {
			return err
		}
		if !access.IsTopic() {
			return ErrTopicInvalidSource
		}
		if access.IsArchived() {
			return ErrTopicArchived
		}
		if _, err := requireActiveTopicUserMember(tx, access, actor.ID); err != nil {
			return err
		}
		if err := conversationaccess.RequireUserDirectAppAccess(tx, access, actor.ID); err != nil {
			if errors.Is(err, conversationaccess.ErrDirectAppAccessDenied) {
				return ErrAccessDenied
			}
			return err
		}
		now := s.now().UTC()
		participant := store.ConversationTopicParticipant{
			ConversationID: topicID, ParticipantType: store.ConversationMemberTypeUser,
			ParticipantID: actor.ID, JoinedReason: store.TopicParticipantReasonManual,
			JoinedAt: now, HistoryVisibleFromSeq: 1, LastReadMessageID: access.Conversation.LastMessageID,
			LastReadSeq: access.Conversation.LastMessageSeq,
			CreatedAt:   now, UpdatedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&participant).Error; err != nil {
			return err
		}
		conversation = access.Conversation
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return Item{}, notFound("话题不存在", err)
		case errors.Is(err, ErrTopicArchived):
			return Item{}, conflict("话题已关闭，不能参与", err)
		case errors.Is(err, ErrAccessDenied):
			return Item{}, forbidden("无权参与话题", err)
		case errors.Is(err, ErrTopicInvalidSource):
			return Item{}, invalidRequest("会话不是话题", err)
		default:
			return Item{}, internalError(err)
		}
	}
	item, err := s.loadItem(s.db.WithContext(ctx), conversation, actor.ID)
	if err != nil {
		return Item{}, internalError(err)
	}
	if s.notifications != nil {
		s.notifications.PublishTopicEvent(ctx, []string{actor.ID}, TopicEvent{
			ConversationID: topicID, ParentConversationID: item.Topic.ParentConversationID,
			SourceMessageID: item.Topic.SourceMessageID, Type: "participated",
		})
	}
	return item, nil
}

func (s *Service) ArchiveTopic(ctx context.Context, cmd ArchiveTopicCommand) (Item, error) {
	topicID, err := normalizeConversationID(cmd.TopicConversationID)
	if err != nil {
		return Item{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	var conversation store.Conversation
	var closedMessage *store.Message
	var appEvents []AppEvent
	appEventLockHeld := false
	topicEventUserIDs := []string{}
	messageUserIDs := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access, err := conversationaccess.Load(tx, topicID, true)
		if err != nil {
			return err
		}
		if !access.IsTopic() {
			return ErrTopicInvalidSource
		}
		member, err := requireActiveTopicUserMember(tx, access, actor.ID)
		if err != nil {
			return err
		}
		if err := conversationaccess.RequireUserDirectAppAccess(tx, access, actor.ID); err != nil {
			if errors.Is(err, conversationaccess.ErrDirectAppAccessDenied) {
				return ErrAccessDenied
			}
			return err
		}
		isSourceSender := access.Topic.SourceSenderType == store.MessageSenderTypeUser &&
			access.Topic.SourceSenderID != nil && *access.Topic.SourceSenderID == actor.ID
		isUserCreator := access.Topic.CreatedByAppID == nil && access.Topic.CreatedByUserID == actor.ID
		if !isUserCreator && !isSourceSender && member.Role != store.ConversationMemberRoleOwner && member.Role != store.ConversationMemberRoleAdmin {
			return ErrAccessDenied
		}
		if access.IsArchived() {
			conversation = access.Conversation
			return nil
		}
		now := s.now().UTC()
		if err := tx.Model(&store.ConversationTopic{}).Where("conversation_id = ?", topicID).Updates(map[string]any{
			"archived_at": now, "archived_by_user_id": actor.ID, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.Conversation{}).Where("id = ?", topicID).Updates(map[string]any{
			"posting_policy": store.ConversationPostingPolicyMuted, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		conversation = access.Conversation
		conversation.PostingPolicy = store.ConversationPostingPolicyMuted
		created, err := createTopicClosedSystemMessage(tx, &conversation, actor, now)
		if err != nil {
			return err
		}
		closedMessage = &created
		if err := conversationaccess.AdvanceTopicParticipantReadSeq(
			tx, topicID, store.ConversationMemberTypeUser, actor.ID,
			created.Seq, &created.ID, now,
		); err != nil {
			return err
		}
		messageUserIDs, err = conversationaccess.ActiveTopicParticipantIDs(tx, access, store.ConversationMemberTypeUser)
		if err != nil {
			return err
		}
		messageUserIDs = appendUniqueString(messageUserIDs, actor.ID)
		members, err := conversationaccess.ActiveMembers(tx, access)
		if err != nil {
			return err
		}
		for _, current := range members {
			if current.MemberType == store.ConversationMemberTypeUser && conversationaccess.TopicSourceVisibleToMember(access, current) {
				topicEventUserIDs = append(topicEventUserIDs, current.MemberID)
			}
		}
		if s.appEvents != nil {
			if s.appEventLocker != nil {
				s.appEventLocker.Lock()
				appEventLockHeld = true
			}
			appEvents, err = createTopicClosedAppEventOutbox(tx, access, now)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if appEventLockHeld {
		defer s.appEventLocker.Unlock()
	}
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return Item{}, notFound("话题不存在", err)
		case errors.Is(err, ErrAccessDenied):
			return Item{}, forbidden("无权关闭话题", err)
		case errors.Is(err, ErrTopicInvalidSource):
			return Item{}, invalidRequest("会话不是话题", err)
		default:
			return Item{}, internalError(err)
		}
	}
	item, err := s.loadItem(s.db.WithContext(ctx), conversation, actor.ID)
	if err != nil {
		return Item{}, internalError(err)
	}
	if s.notifications != nil {
		if closedMessage != nil {
			s.notifications.PublishConversationMessage(ctx, messageUserIDs, newMessage(*closedMessage))
		}
		s.notifications.PublishTopicEvent(ctx, topicEventUserIDs, TopicEvent{
			Archived: true, ConversationID: topicID, ParentConversationID: item.Topic.ParentConversationID,
			SourceMessageID: item.Topic.SourceMessageID, Type: "archived",
		})
	}
	if closedMessage != nil && s.appEvents != nil {
		s.appEvents.DeliverConversationAppEvents(ctx, appEvents)
	}
	return item, nil
}

func (s *Service) CloseTopicAsApp(ctx context.Context, cmd AppCloseTopicCommand) (AppTopicResult, error) {
	appID, err := normalizeUUID(cmd.AppID, "应用 ID 格式错误")
	if err != nil {
		return AppTopicResult{}, invalidRequest(err.Error(), err)
	}
	topicID, err := normalizeConversationID(cmd.TopicConversationID)
	if err != nil {
		return AppTopicResult{}, invalidRequest(err.Error(), err)
	}
	if cmd.ExpectedLastMessageSeq < 0 {
		return AppTopicResult{}, invalidRequest("expected_last_message_seq 不能小于 0", ErrTopicChanged)
	}
	var actor store.App
	var conversation store.Conversation
	var topic store.ConversationTopic
	var closedMessage *store.Message
	var appEvents []AppEvent
	appEventLockHeld := false
	messageUserIDs := []string{}
	topicEventUserIDs := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&actor, "id = ? AND enabled = ?", appID, true).Error; err != nil {
			return err
		}
		access, err := conversationaccess.Load(tx, topicID, true)
		if err != nil {
			return err
		}
		if !access.IsTopic() {
			return ErrTopicInvalidSource
		}
		if _, err := requireActiveTopicActorMember(tx, access, store.ConversationMemberTypeApp, appID); err != nil {
			return err
		}
		if err := conversationaccess.RequireAppDirectUserAccess(tx, access, appID); err != nil {
			if errors.Is(err, conversationaccess.ErrDirectAppAccessDenied) {
				return ErrAccessDenied
			}
			return err
		}
		if access.Topic.CreatedByAppID == nil || *access.Topic.CreatedByAppID != appID {
			return ErrAccessDenied
		}
		conversation = access.Conversation
		topic = *access.Topic
		if access.IsArchived() {
			return nil
		}
		if conversation.LastMessageSeq != cmd.ExpectedLastMessageSeq {
			return ErrTopicChanged
		}
		now := s.now().UTC()
		if err := tx.Model(&store.ConversationTopic{}).Where("conversation_id = ?", topicID).Updates(map[string]any{
			"archived_at": now, "archived_by_app_id": appID, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.Conversation{}).Where("id = ?", topicID).Updates(map[string]any{
			"posting_policy": store.ConversationPostingPolicyMuted, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		conversation.PostingPolicy = store.ConversationPostingPolicyMuted
		created, err := createTopicClosedByAppSystemMessage(tx, &conversation, actor, now)
		if err != nil {
			return err
		}
		closedMessage = &created
		topic.ArchivedAt = &now
		topic.ArchivedByAppID = &appID
		messageUserIDs, err = conversationaccess.ActiveTopicParticipantIDs(tx, access, store.ConversationMemberTypeUser)
		if err != nil {
			return err
		}
		members, err := conversationaccess.ActiveMembers(tx, access)
		if err != nil {
			return err
		}
		for _, current := range members {
			if current.MemberType == store.ConversationMemberTypeUser && conversationaccess.TopicSourceVisibleToMember(access, current) {
				topicEventUserIDs = append(topicEventUserIDs, current.MemberID)
			}
		}
		if s.appEvents != nil {
			if s.appEventLocker != nil {
				s.appEventLocker.Lock()
				appEventLockHeld = true
			}
			appEvents, err = createTopicClosedAppEventOutbox(tx, access, now)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if appEventLockHeld {
		defer s.appEventLocker.Unlock()
	}
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return AppTopicResult{}, notFound("话题或应用不存在", err)
		case errors.Is(err, ErrTopicChanged):
			return AppTopicResult{}, conflict("话题已有新消息", err)
		case errors.Is(err, ErrAccessDenied):
			return AppTopicResult{}, forbidden("应用无权关闭话题", err)
		case errors.Is(err, ErrTopicInvalidSource):
			return AppTopicResult{}, invalidRequest("会话不是话题", err)
		default:
			return AppTopicResult{}, internalError(err)
		}
	}
	if s.notifications != nil {
		if closedMessage != nil {
			s.notifications.PublishConversationMessage(ctx, messageUserIDs, newMessage(*closedMessage))
		}
		s.notifications.PublishTopicEvent(ctx, topicEventUserIDs, TopicEvent{
			Archived: true, ConversationID: topicID, ParentConversationID: topic.ParentConversationID,
			SourceMessageID: topic.SourceMessageID, Type: "archived",
		})
	}
	if closedMessage != nil && s.appEvents != nil {
		s.appEvents.DeliverConversationAppEvents(ctx, appEvents)
	}
	return newAppTopicResult(conversation, topic, false), nil
}

func (s *Service) GetTopicAsApp(ctx context.Context, cmd AppGetTopicCommand) (AppTopicResult, error) {
	appID, err := normalizeUUID(cmd.AppID, "应用 ID 格式错误")
	if err != nil {
		return AppTopicResult{}, invalidRequest(err.Error(), err)
	}
	topicID, err := normalizeConversationID(cmd.TopicConversationID)
	if err != nil {
		return AppTopicResult{}, invalidRequest(err.Error(), err)
	}
	db := s.db.WithContext(ctx)
	access, err := conversationaccess.Load(db, topicID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AppTopicResult{}, notFound("话题不存在", err)
		}
		return AppTopicResult{}, internalError(err)
	}
	if !access.IsTopic() {
		return AppTopicResult{}, invalidRequest("会话不是话题", ErrTopicInvalidSource)
	}
	if _, err := requireActiveTopicActorMember(db, access, store.ConversationMemberTypeApp, appID); err != nil {
		return AppTopicResult{}, forbidden("应用无权访问话题", err)
	}
	if _, err := conversationaccess.TopicParticipant(db, topicID, store.ConversationMemberTypeApp, appID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AppTopicResult{}, forbidden("应用无权访问话题", ErrAccessDenied)
		}
		return AppTopicResult{}, internalError(err)
	}
	return newAppTopicResult(access.Conversation, *access.Topic, false), nil
}

func appendUniqueString(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func requireTopicCreatorDirectAppAccess(db *gorm.DB, access conversationaccess.Context, creator topicCreator) error {
	var err error
	if creator.memberType == store.ConversationMemberTypeApp {
		err = conversationaccess.RequireAppDirectUserAccess(db, access, creator.id)
	} else {
		err = conversationaccess.RequireUserDirectAppAccess(db, access, creator.id)
	}
	if errors.Is(err, conversationaccess.ErrDirectAppAccessDenied) {
		return ErrAccessDenied
	}
	return err
}

func loadTopicSourceMessage(db *gorm.DB, conversationID, messageID string) (store.Message, error) {
	if store.MessagePartitioningEnabled(db) {
		var registry store.MessageRegistry
		if err := db.First(&registry, "id = ? AND conversation_id = ?", messageID, conversationID).Error; err != nil {
			return store.Message{}, err
		}
		loadContext := db.Statement.Context
		if loadContext == nil {
			loadContext = context.Background()
		}
		return store.LoadMessageByRegistry(loadContext, db, registry)
	}
	var message store.Message
	err := db.First(&message, "id = ? AND conversation_id = ?", messageID, conversationID).Error
	return message, err
}

func loadTopicSourceSenderName(db *gorm.DB, message store.Message) (string, error) {
	if message.SenderID == nil {
		return "", gorm.ErrRecordNotFound
	}
	switch message.SenderType {
	case store.MessageSenderTypeUser:
		var user store.User
		if err := db.Select("id", "name", "nickname").First(&user, "id = ?", *message.SenderID).Error; err != nil {
			return "", err
		}
		return userDisplayName(user), nil
	case store.MessageSenderTypeApp:
		var app store.App
		if err := db.Unscoped().Select("id", "name").First(&app, "id = ?", *message.SenderID).Error; err != nil {
			return "", err
		}
		return strings.TrimSpace(app.Name), nil
	default:
		return "", ErrTopicInvalidSource
	}
}

func loadTopicSourceSenderAvatar(db *gorm.DB, senderType string, senderID *string) (string, error) {
	if senderID == nil {
		return "", nil
	}
	switch senderType {
	case store.MessageSenderTypeUser:
		var user store.User
		result := db.Select("id", "avatar").Where("id = ?", *senderID).Limit(1).Find(&user)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected == 0 {
			return "", nil
		}
		if strings.TrimSpace(user.Avatar) == "" {
			return store.DefaultUserAvatar, nil
		}
		return user.Avatar, nil
	case store.MessageSenderTypeApp:
		var app store.App
		result := db.Unscoped().Select("id", "avatar").Where("id = ?", *senderID).Limit(1).Find(&app)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected == 0 {
			return "", nil
		}
		return app.Avatar, nil
	default:
		return "", nil
	}
}

func resolveTopicName(db *gorm.DB, summary string) (string, error) {
	matches := topicMentionTokenPattern.FindAllStringSubmatch(summary, -1)
	if len(matches) == 0 {
		return normalizeTopicName(summary), nil
	}
	userSet := make(map[string]struct{})
	appSet := make(map[string]struct{})
	for _, match := range matches {
		if len(match) != 5 || match[2] == "all" {
			continue
		}
		id := strings.ToLower(match[4])
		if match[3] == store.ConversationMemberTypeApp {
			appSet[id] = struct{}{}
		} else {
			userSet[id] = struct{}{}
		}
	}
	userNames := make(map[string]string, len(userSet))
	if userIDs := sortedKeys(userSet); len(userIDs) > 0 {
		var users []store.User
		if err := db.Select("id", "name", "nickname").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return "", err
		}
		for _, user := range users {
			userNames[user.ID] = userDisplayName(user)
		}
	}
	appNames := make(map[string]string, len(appSet))
	if appIDs := sortedKeys(appSet); len(appIDs) > 0 {
		var apps []store.App
		if err := db.Unscoped().Select("id", "name").Where("id IN ?", appIDs).Find(&apps).Error; err != nil {
			return "", err
		}
		for _, app := range apps {
			if name := strings.TrimSpace(app.Name); name != "" {
				appNames[app.ID] = name
			}
		}
	}
	resolved := topicMentionTokenPattern.ReplaceAllStringFunc(summary, func(token string) string {
		match := topicMentionTokenPattern.FindStringSubmatch(token)
		if len(match) != 5 {
			return token
		}
		if match[2] == "all" {
			return "@所有人"
		}
		id := strings.ToLower(match[4])
		if match[3] == store.ConversationMemberTypeApp {
			if name := appNames[id]; name != "" {
				return "@" + name
			}
			return "@应用"
		}
		if name := userNames[id]; name != "" {
			return "@" + name
		}
		return "@用户"
	})
	return normalizeTopicName(resolved), nil
}

func normalizeTopicName(summary string) string {
	value := strings.Join(strings.Fields(summary), " ")
	if value == "" {
		return "话题"
	}
	if utf8.RuneCountInString(value) <= maxTopicNameLength {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxTopicNameLength])
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func requireActiveTopicUserMember(db *gorm.DB, access conversationaccess.Context, userID string) (store.ConversationMember, error) {
	if !access.IsTopic() || access.Conversation.Status != store.ConversationStatusActive ||
		access.ParentConversation == nil || access.ParentConversation.Status != store.ConversationStatusActive {
		return store.ConversationMember{}, ErrAccessDenied
	}
	member, err := conversationaccess.RequireUserMember(db, access, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.ConversationMember{}, ErrAccessDenied
		}
		return store.ConversationMember{}, err
	}
	if !conversationaccess.TopicSourceVisibleToMember(access, member) {
		return store.ConversationMember{}, ErrAccessDenied
	}
	return member, nil
}

func requireActiveTopicActorMember(db *gorm.DB, access conversationaccess.Context, memberType, memberID string) (store.ConversationMember, error) {
	if !access.IsTopic() || access.Conversation.Status != store.ConversationStatusActive ||
		access.ParentConversation == nil || access.ParentConversation.Status != store.ConversationStatusActive {
		return store.ConversationMember{}, ErrAccessDenied
	}
	var member store.ConversationMember
	var err error
	switch memberType {
	case store.ConversationMemberTypeUser:
		member, err = conversationaccess.RequireUserMember(db, access, memberID)
	case store.ConversationMemberTypeApp:
		member, err = conversationaccess.RequireAppMember(db, access, memberID)
	default:
		return store.ConversationMember{}, ErrAccessDenied
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.ConversationMember{}, ErrAccessDenied
		}
		return store.ConversationMember{}, err
	}
	if !conversationaccess.TopicSourceVisibleToMember(access, member) {
		return store.ConversationMember{}, ErrAccessDenied
	}
	return member, nil
}

func requireTopicCreatorMember(db *gorm.DB, access conversationaccess.Context, creator topicCreator) (store.ConversationMember, error) {
	switch creator.memberType {
	case store.ConversationMemberTypeUser:
		return conversationaccess.RequireUserMember(db, access, creator.id)
	case store.ConversationMemberTypeApp:
		var app store.App
		if err := db.Select("id").First(&app, "id = ? AND enabled = ?", creator.id, true).Error; err != nil {
			return store.ConversationMember{}, err
		}
		return conversationaccess.RequireAppMember(db, access, creator.id)
	default:
		return store.ConversationMember{}, ErrAccessDenied
	}
}

func resolveTopicLegacyCreatorUserID(creator topicCreator, source store.Message, members []store.ConversationMember) (string, error) {
	if creator.memberType == store.ConversationMemberTypeUser {
		return creator.id, nil
	}
	if source.SenderType == store.MessageSenderTypeUser && source.SenderID != nil {
		return *source.SenderID, nil
	}
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeUser {
			return member.MemberID, nil
		}
	}
	return "", ErrAccessDenied
}

func initialTopicParticipants(parent store.Conversation, source store.Message, members []store.ConversationMember, creator topicCreator, conversationID string, now time.Time) []store.ConversationTopicParticipant {
	mentioned := topicMentionedMemberKeys(source.Summary)
	participants := make([]store.ConversationTopicParticipant, 0, len(members))
	for _, member := range members {
		if !conversationaccess.SourceMessageVisibleToMember(source.Seq, member) {
			continue
		}
		key := memberKey(member.MemberType, member.MemberID)
		include := parent.Kind != store.ConversationKindGroup ||
			(member.MemberType == creator.memberType && member.MemberID == creator.id) ||
			(source.SenderID != nil && member.MemberType == source.SenderType && member.MemberID == *source.SenderID)
		if _, ok := mentioned[key]; ok {
			include = true
		}
		if _, ok := mentioned[memberKey(store.ConversationMemberTypeUser, "all")]; ok && member.MemberType == store.ConversationMemberTypeUser {
			include = true
		}
		if !include {
			continue
		}
		reason := store.TopicParticipantReasonAutomatic
		if member.MemberType == creator.memberType && member.MemberID == creator.id {
			reason = store.TopicParticipantReasonCreator
		} else if _, ok := mentioned[key]; ok {
			reason = store.TopicParticipantReasonMention
		}
		participants = append(participants, store.ConversationTopicParticipant{
			ConversationID: conversationID, ParticipantType: member.MemberType,
			ParticipantID: member.MemberID, JoinedReason: reason, JoinedAt: now,
			HistoryVisibleFromSeq: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	return participants
}

func topicMentionedMemberKeys(summary string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, match := range topicMentionTokenPattern.FindAllStringSubmatch(summary, -1) {
		if len(match) != 5 {
			continue
		}
		if match[2] == "all" {
			result[memberKey(store.ConversationMemberTypeUser, "all")] = struct{}{}
			continue
		}
		result[memberKey(strings.ToLower(match[3]), strings.ToLower(match[4]))] = struct{}{}
	}
	return result
}

func newAppTopicResult(conversation store.Conversation, topic store.ConversationTopic, created bool) AppTopicResult {
	return AppTopicResult{
		Archived: topic.ArchivedAt != nil, ConversationID: conversation.ID, Created: created,
		LastMessageSeq: conversation.LastMessageSeq, Name: conversation.Name,
		ParentConversationID: topic.ParentConversationID, SourceMessageID: topic.SourceMessageID,
		Type: conversation.Kind,
	}
}
