package message

import (
	"encoding/json"
	"regexp"
	"strings"

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

func updateConversationMentionedSeq(db *gorm.DB, conversationKind, conversationID string, seq int64, body json.RawMessage) ([]string, error) {
	if conversationKind != store.ConversationKindGroup {
		return nil, nil
	}
	targets := parseMessageMentionTargets(body)
	if len(targets) == 0 {
		return nil, nil
	}
	var members []store.ConversationMember
	if err := db.Where("conversation_id = ? AND left_at IS NULL", conversationID).Find(&members).Error; err != nil {
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
	mentioned := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, member := range members {
		_, direct := targetSet[mentionKey(member.MemberType, member.MemberID)]
		byAll := mentionAll && member.MemberType == store.ConversationMemberTypeUser
		if !direct && !byAll {
			continue
		}
		if err := db.Model(&store.ConversationMember{}).
			Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, member.MemberType, member.MemberID).
			Update("last_mentioned_seq", gorm.Expr("CASE WHEN last_mentioned_seq > ? THEN last_mentioned_seq ELSE ? END", seq, seq)).Error; err != nil {
			return nil, err
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
		Type string `json:"type"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return "", false
	}
	if strings.TrimSpace(envelope.Type) != "text" && strings.TrimSpace(envelope.Type) != "markdown" {
		return "", false
	}
	var value struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(body, &value) != nil {
		return "", false
	}
	return value.Content, true
}

func mentionKey(memberType, memberID string) string {
	return memberType + "/" + memberID
}
