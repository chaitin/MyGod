package conversation

import (
	"encoding/json"
	"strings"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	messageTypeSystemEvent              = "system_event"
	systemEventGroupMembersInvited      = "group_members_invited"
	systemEventGroupAvatarUpdated       = "group_avatar_updated"
	systemEventGroupVisibilityChanged   = "group_visibility_changed"
	systemEventGroupMemberJoined        = "group_member_joined"
	systemEventGroupMemberLeft          = "group_member_left"
	systemEventGroupMemberRemoved       = "group_member_removed"
	systemEventGroupNameUpdated         = "group_name_updated"
	groupMembersInvitedSummarySeparator = ","
)

type systemEventUserRef struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
	Type        string `json:"type,omitempty"`
}

type groupMembersInvitedSystemEventBody struct {
	Event    string               `json:"event"`
	Invitees []systemEventUserRef `json:"invitees"`
	Inviter  systemEventUserRef   `json:"inviter"`
	Type     string               `json:"type"`
}

type groupAvatarUpdatedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

type groupVisibilityChangedSystemEventBody struct {
	Actor      systemEventUserRef `json:"actor"`
	Event      string             `json:"event"`
	Type       string             `json:"type"`
	Visibility string             `json:"visibility"`
}

type groupMemberJoinedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

type groupMemberLeftSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

type groupMemberRemovedSystemEventBody struct {
	Actor  systemEventUserRef `json:"actor"`
	Event  string             `json:"event"`
	Target systemEventUserRef `json:"target"`
	Type   string             `json:"type"`
}

type groupNameUpdatedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Name  string             `json:"name"`
	Type  string             `json:"type"`
}

func createGroupMembersInvitedSystemMessage(db *gorm.DB, conversation *store.Conversation, inviter store.User, invitees []systemEventUserRef, now time.Time) (store.Message, error) {
	names := make([]string, 0, len(invitees))
	for _, invitee := range invitees {
		names = append(names, invitee.DisplayName)
	}
	displayName := userDisplayName(inviter)
	body, err := json.Marshal(groupMembersInvitedSystemEventBody{
		Event: systemEventGroupMembersInvited, Invitees: invitees,
		Inviter: systemEventUserRef{DisplayName: displayName, ID: inviter.ID}, Type: messageTypeSystemEvent,
	})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, displayName+" 邀请 "+strings.Join(names, groupMembersInvitedSummarySeparator)+" 加入群聊", now)
}

func createGroupAvatarUpdatedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	displayName := userDisplayName(actor)
	body, err := json.Marshal(groupAvatarUpdatedSystemEventBody{Actor: systemEventUserRef{DisplayName: displayName, ID: actor.ID}, Event: systemEventGroupAvatarUpdated, Type: messageTypeSystemEvent})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, displayName+" 修改了群头像", now)
}

func createGroupVisibilityChangedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, visibility string, now time.Time) (store.Message, error) {
	displayName := userDisplayName(actor)
	summary := displayName + " 将当前群设置为公开群"
	if visibility == store.ConversationVisibilityPrivate {
		summary = displayName + " 将当前群设为私有群"
	}
	body, err := json.Marshal(groupVisibilityChangedSystemEventBody{Actor: systemEventUserRef{DisplayName: displayName, ID: actor.ID}, Event: systemEventGroupVisibilityChanged, Type: messageTypeSystemEvent, Visibility: visibility})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, summary, now)
}

func createGroupMemberJoinedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	displayName := userDisplayName(actor)
	body, err := json.Marshal(groupMemberJoinedSystemEventBody{Actor: systemEventUserRef{DisplayName: displayName, ID: actor.ID}, Event: systemEventGroupMemberJoined, Type: messageTypeSystemEvent})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, displayName+" 加入群聊", now)
}

func createGroupMemberLeftSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	displayName := userDisplayName(actor)
	body, err := json.Marshal(groupMemberLeftSystemEventBody{Actor: systemEventUserRef{DisplayName: displayName, ID: actor.ID}, Event: systemEventGroupMemberLeft, Type: messageTypeSystemEvent})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, displayName+" 已退出群聊", now)
}

func createGroupMemberRemovedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, target systemEventUserRef, now time.Time) (store.Message, error) {
	displayName := userDisplayName(actor)
	body, err := json.Marshal(groupMemberRemovedSystemEventBody{Actor: systemEventUserRef{DisplayName: displayName, ID: actor.ID}, Event: systemEventGroupMemberRemoved, Target: target, Type: messageTypeSystemEvent})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, displayName+" 已将 "+target.DisplayName+" 移出群聊", now)
}

func createGroupNameUpdatedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, name string, now time.Time) (store.Message, error) {
	displayName := userDisplayName(actor)
	body, err := json.Marshal(groupNameUpdatedSystemEventBody{Actor: systemEventUserRef{DisplayName: displayName, ID: actor.ID}, Event: systemEventGroupNameUpdated, Name: name, Type: messageTypeSystemEvent})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, displayName+" 修改群聊名称为 "+name, now)
}

func createSystemMessage(db *gorm.DB, conversation *store.Conversation, body json.RawMessage, summary string, now time.Time) (store.Message, error) {
	message := store.Message{ID: uuid.NewString(), ConversationID: conversation.ID, Seq: conversation.LastMessageSeq + 1, SenderType: store.MessageSenderTypeSystem, Body: body, Summary: summary, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(map[string]any{
		"last_message_at": message.CreatedAt, "last_message_id": message.ID, "last_message_seq": message.Seq,
		"last_message_summary": message.Summary, "updated_at": now,
	}).Error; err != nil {
		return store.Message{}, err
	}
	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now
	return message, nil
}

func makeInviteeRefs(users []store.User, apps []store.App) []systemEventUserRef {
	refs := make([]systemEventUserRef, 0, len(users)+len(apps))
	for _, user := range users {
		refs = append(refs, systemEventUserRef{DisplayName: userDisplayName(user), ID: user.ID})
	}
	for _, app := range apps {
		refs = append(refs, systemEventUserRef{DisplayName: app.Name, ID: app.ID, Type: store.ConversationMemberTypeApp})
	}
	return refs
}

func loadMemberSystemRef(db *gorm.DB, memberType, memberID string) (systemEventUserRef, error) {
	switch memberType {
	case store.ConversationMemberTypeUser:
		var user store.User
		if err := db.First(&user, "id = ?", memberID).Error; err != nil {
			return systemEventUserRef{}, err
		}
		return systemEventUserRef{DisplayName: userDisplayName(user), ID: user.ID}, nil
	case store.ConversationMemberTypeApp:
		var app store.App
		if err := db.Unscoped().First(&app, "id = ?", memberID).Error; err != nil {
			return systemEventUserRef{}, err
		}
		return systemEventUserRef{DisplayName: app.Name, ID: app.ID, Type: store.ConversationMemberTypeApp}, nil
	default:
		return systemEventUserRef{}, gorm.ErrRecordNotFound
	}
}
