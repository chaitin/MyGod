package httpserver

import (
	"testing"

	"app/internal/store"
)

func TestNewConversationListItemResponseUsesDirectFallbackCopy(t *testing.T) {
	currentUserID := "user-1"

	response := newConversationListItemResponse(
		store.Conversation{
			ID:   "conversation-1",
			Kind: store.ConversationKindDirect,
		},
		currentUserID,
		[]store.ConversationMember{
			{
				ConversationID: "conversation-1",
				MemberType:     store.ConversationMemberTypeUser,
				MemberID:       currentUserID,
				Role:           store.ConversationMemberRoleOwner,
			},
		},
		map[string]store.User{},
		nil,
	)

	if response.Name != "私聊" {
		t.Fatalf("response.Name = %q, want %q", response.Name, "私聊")
	}
}
