package message

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"app/internal/application/conversationaccess"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxMessageMentionTargets = 50

var messageMentionTokenPattern = regexp.MustCompile(`\{\(@(?:(user)/(all)|(user|app)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))\)\}`)

type messageMentionTarget struct {
	All        bool
	MemberID   string
	MemberType string
}

func updateConversationMentionedSeq(db *gorm.DB, access conversationaccess.Context, seq int64, body json.RawMessage, now time.Time) ([]string, error) {
	effective := access.EffectiveConversation()
	if effective.Kind != store.ConversationKindGroup {
		return nil, nil
	}
	targets := parseMessageMentionTargets(body)
	if len(targets) == 0 {
		return nil, nil
	}
	var members []store.ConversationMember
	if err := db.Where("conversation_id = ? AND left_at IS NULL", access.MembershipConversationID).Find(&members).Error; err != nil {
		return nil, err
	}
	mentionAll := false
	targetSet := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target.All {
			mentionAll = true
			continue
		}
		targetSet[mentionKey(target.MemberType, target.MemberID)] = struct{}{}
	}
	topicParticipantSet := map[string]struct{}{}
	if access.IsTopic() && mentionAll {
		var participants []store.ConversationTopicParticipant
		if err := db.Select("participant_type", "participant_id").Where(
			"conversation_id = ?", access.Conversation.ID,
		).Find(&participants).Error; err != nil {
			return nil, err
		}
		for _, participant := range participants {
			topicParticipantSet[mentionKey(participant.ParticipantType, participant.ParticipantID)] = struct{}{}
		}
	}
	mentioned := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, member := range members {
		if !conversationaccess.TopicSourceVisibleToMember(access, member) {
			continue
		}
		_, direct := targetSet[mentionKey(member.MemberType, member.MemberID)]
		byAll := mentionAll && member.MemberType == store.ConversationMemberTypeUser
		if access.IsTopic() && byAll {
			_, byAll = topicParticipantSet[mentionKey(member.MemberType, member.MemberID)]
		}
		if !direct && !byAll {
			continue
		}
		if access.IsTopic() {
			if err := conversationaccess.EnsureTopicParticipant(
				db, access, member.MemberType, member.MemberID,
				store.TopicParticipantReasonMention, 0, seq, now,
			); err != nil {
				return nil, err
			}
		} else {
			if err := db.Model(&store.ConversationMember{}).
				Where("conversation_id = ? AND member_type = ? AND member_id = ?", access.Conversation.ID, member.MemberType, member.MemberID).
				Update("last_mentioned_seq", gorm.Expr("CASE WHEN last_mentioned_seq > ? THEN last_mentioned_seq ELSE ? END", seq, seq)).Error; err != nil {
				return nil, err
			}
		}
		if member.MemberType == store.ConversationMemberTypeUser {
			if _, ok := seen[member.MemberID]; ok {
				continue
			}
			seen[member.MemberID] = struct{}{}
			mentioned = append(mentioned, member.MemberID)
		}
	}
	return mentioned, nil
}

func parseMessageMentionTargets(body json.RawMessage) []messageMentionTarget {
	content, ok := messageMentionContent(body)
	if !ok {
		return nil
	}
	matches := messageMentionTokenPattern.FindAllStringSubmatch(content, -1)
	result := make([]messageMentionTarget, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) != 5 {
			continue
		}
		if match[2] == "all" {
			if _, ok := seen["all"]; ok {
				continue
			}
			seen["all"] = struct{}{}
			result = append(result, messageMentionTarget{All: true})
		} else {
			id, err := uuid.Parse(match[4])
			if err != nil {
				continue
			}
			target := messageMentionTarget{MemberID: id.String(), MemberType: match[3]}
			key := mentionKey(target.MemberType, target.MemberID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, target)
		}
		if len(result) >= maxMessageMentionTargets {
			break
		}
	}
	return result
}

func messageMentionContent(body json.RawMessage) (string, bool) {
	var envelope struct {
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		Type        string `json:"type"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return "", false
	}
	messageType := strings.TrimSpace(envelope.Type)
	if messageType == "choice" {
		contentType := strings.TrimSpace(envelope.ContentType)
		if contentType != "text" && contentType != "markdown" {
			return "", false
		}
		return envelope.Content, true
	}
	if messageType != "text" && messageType != "markdown" {
		return "", false
	}
	return envelope.Content, true
}

func mentionKey(memberType, memberID string) string {
	return memberType + "/" + memberID
}
