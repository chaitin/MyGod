package conversation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	fileapp "app/internal/application/file"
	"app/internal/media"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) AuthorizeAvatarUpdate(ctx context.Context, cmd AuthorizeAvatarCommand) (AvatarUploadAuthorization, error) {
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return AvatarUploadAuthorization{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	if err := s.requireAvatarPermission(s.db, actor.ID, conversationID); err != nil {
		return AvatarUploadAuthorization{}, mapAvatarPermissionError(err)
	}
	return AvatarUploadAuthorization{actor: cmd.Actor, conversationID: conversationID, valid: true}, nil
}

func (s *Service) UploadAvatar(ctx context.Context, cmd UploadAvatarCommand) (UpdateAvatarResult, error) {
	if !cmd.Authorization.valid {
		return UpdateAvatarResult{}, internalError(errors.New("avatar upload authorization required"))
	}
	conversationID := cmd.Authorization.conversationID
	actor := actorUser(cmd.Authorization.actor)
	if cmd.Size > MaxAvatarUploadBytes {
		return UpdateAvatarResult{}, newError(CodeRequestTooLarge, "群头像文件不能超过 1MiB", nil)
	}
	if cmd.Size == 0 || cmd.Content == nil {
		return UpdateAvatarResult{}, invalidRequest("群头像文件不能为空", nil)
	}
	content, err := io.ReadAll(io.LimitReader(cmd.Content, MaxAvatarUploadBytes+1))
	if err != nil {
		return UpdateAvatarResult{}, invalidRequest("读取群头像失败", err)
	}
	if len(content) > MaxAvatarUploadBytes {
		return UpdateAvatarResult{}, newError(CodeRequestTooLarge, "群头像文件不能超过 1MiB", nil)
	}
	if len(content) == 0 {
		return UpdateAvatarResult{}, invalidRequest("群头像文件不能为空", nil)
	}
	width, height, err := media.WebPDimensions(content)
	if err != nil || width != 256 || height != 256 {
		return UpdateAvatarResult{}, invalidRequest("群头像必须是 256x256 的 WebP 图片", err)
	}
	if s.files == nil {
		return UpdateAvatarResult{}, newError(CodeInternal, "群头像存储未配置", nil)
	}
	key := fmt.Sprintf("avatars/conversations/%s/%s.webp", strings.TrimSpace(conversationID), uuid.NewString())
	uploaded, err := s.files.UploadPublic(ctx, fileapp.UploadPublicCommand{ObjectKey: key, Content: bytes.NewReader(content), ContentType: AvatarContentType, SizeBytes: int64(len(content))})
	if err != nil {
		if fileapp.ErrorCodeOf(err) == fileapp.CodeStorageUnavailable {
			return UpdateAvatarResult{}, newError(CodeInternal, "群头像存储未配置", err)
		}
		return UpdateAvatarResult{}, newError(CodeInternal, "上传群头像失败", err)
	}
	conversation, message, userIDs, err := s.updateAvatar(s.db, actor, conversationID, uploaded.URL)
	if err != nil {
		return UpdateAvatarResult{}, mapAvatarPermissionError(err)
	}
	item, err := s.loadItem(s.db, conversation, actor.ID)
	if err != nil {
		return UpdateAvatarResult{}, internalError(err)
	}
	resultMessage := newMessage(message)
	if s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, resultMessage)
	}
	return UpdateAvatarResult{Conversation: item, Message: resultMessage}, nil
}

func mapAvatarPermissionError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return notFound("会话不存在", err)
	case errors.Is(err, ErrAccessDenied):
		return forbidden("无权访问会话", err)
	case errors.Is(err, ErrNotGroup):
		return invalidRequest("只能修改群聊头像", err)
	case errors.Is(err, ErrAvatarForbidden):
		return forbidden("只有群主或管理员可以修改群头像", err)
	default:
		return internalError(err)
	}
}

func (s *Service) requireAvatarPermission(db *gorm.DB, userID, conversationID string) error {
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return err
	}
	if conversation.Status != store.ConversationStatusActive {
		return ErrAccessDenied
	}
	if conversation.Kind != store.ConversationKindGroup {
		return ErrNotGroup
	}
	var member store.ConversationMember
	if err := db.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccessDenied
		}
		return err
	}
	if !canManage(member.Role) {
		return ErrAvatarForbidden
	}
	return nil
}

func (s *Service) updateAvatar(db *gorm.DB, actor store.User, conversationID, avatarURL string) (store.Conversation, store.Message, []string, error) {
	var conversation store.Conversation
	var message store.Message
	userIDs := []string{}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return ErrAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return ErrNotGroup
		}
		var current store.ConversationMember
		if err := tx.First(&current, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, actor.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessDenied
			}
			return err
		}
		if !canManage(current.Role) {
			return ErrAvatarForbidden
		}
		now := s.now().UTC()
		if err := tx.Model(&store.Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{"avatar": avatarURL, "updated_at": now}).Error; err != nil {
			return err
		}
		conversation.Avatar, conversation.UpdatedAt = avatarURL, now
		created, err := createGroupAvatarUpdatedSystemMessage(tx, &conversation, actor, now)
		if err != nil {
			return err
		}
		message = created
		if err := advanceReadSeq(tx, conversationID, actor.ID, created.Seq); err != nil {
			return err
		}
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, store.Message{}, nil, err
	}
	return conversation, message, userIDs, nil
}
