package conversation

import (
	"context"
	"errors"

	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) MarkRead(ctx context.Context, cmd ReadCommand) (ReadResult, error) {
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return ReadResult{}, invalidRequest(err.Error(), err)
	}
	if cmd.UpToSeq != nil && *cmd.UpToSeq <= 0 {
		return ReadResult{}, invalidRequest("up_to_seq 必须是正整数", nil)
	}
	result, err := s.markRead(s.db, cmd.AccountID, conversationID, cmd.UpToSeq)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReadResult{}, notFound("会话不存在", err)
	}
	if errors.Is(err, ErrAccessDenied) {
		return ReadResult{}, forbidden("无权访问会话", err)
	}
	if err != nil {
		return ReadResult{}, internalError(err)
	}
	return result, nil
}

func (s *Service) markRead(db *gorm.DB, userID, conversationID string, upToSeq *int64) (ReadResult, error) {
	var response ReadResult
	err := db.Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return ErrAccessDenied
		}
		var member store.ConversationMember
		if err := tx.First(&member, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessDenied
			}
			return err
		}
		targetSeq := conversation.LastMessageSeq
		if upToSeq != nil && *upToSeq < targetSeq {
			targetSeq = *upToSeq
		}
		if err := advanceReadSeq(tx, conversationID, userID, targetSeq); err != nil {
			return err
		}
		if targetSeq > member.LastReadSeq {
			member.LastReadSeq = targetSeq
		}
		response = ReadResult{ConversationID: conversationID, LastReadSeq: member.LastReadSeq, UnreadCount: unreadCount(conversation.LastMessageSeq, member.LastReadSeq)}
		return nil
	})
	return response, err
}
