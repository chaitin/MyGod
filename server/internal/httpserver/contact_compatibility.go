package httpserver

import (
	contactapp "app/internal/application/contact"
)

// These DTOs remain while the App WebSocket transport still lives in the
// legacy httpserver package. Contact persistence lives in the application service.
type contactAppResponse struct {
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Online      bool   `json:"online"`
	Type        string `json:"type"`
}

type contactGroupResponse struct {
	Avatar        string                             `json:"avatar"`
	AvatarMembers []contactGroupAvatarMemberResponse `json:"avatar_members,omitempty"`
	ID            string                             `json:"id"`
	Joined        bool                               `json:"joined"`
	MemberCount   int                                `json:"member_count"`
	Name          string                             `json:"name"`
	Type          string                             `json:"type"`
	Visibility    string                             `json:"visibility"`
}

type contactGroupAvatarMemberResponse struct {
	Avatar   string `json:"avatar"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

type contactUserResponse struct {
	Avatar       string  `json:"avatar"`
	Email        string  `json:"email"`
	ID           string  `json:"id"`
	LastOnlineAt *string `json:"last_online_at"`
	Name         string  `json:"name"`
	Nickname     string  `json:"nickname"`
	Online       bool    `json:"online"`
	Phone        string  `json:"phone"`
	Type         string  `json:"type"`
}

func legacyContactApps(values []contactapp.App) []contactAppResponse {
	result := make([]contactAppResponse, 0, len(values))
	for _, value := range values {
		result = append(result, contactAppResponse{
			Avatar: value.Avatar, Description: value.Description, ID: value.ID,
			Name: value.Name, Online: value.Online, Type: value.Type,
		})
	}
	return result
}

func legacyContactGroups(values []contactapp.Group) []contactGroupResponse {
	result := make([]contactGroupResponse, 0, len(values))
	for _, value := range values {
		var avatarMembers []contactGroupAvatarMemberResponse
		if value.AvatarMembers != nil {
			avatarMembers = make([]contactGroupAvatarMemberResponse, 0, len(value.AvatarMembers))
			for _, member := range value.AvatarMembers {
				avatarMembers = append(avatarMembers, contactGroupAvatarMemberResponse{
					Avatar: member.Avatar, Name: member.Name, Nickname: member.Nickname, Role: member.Role,
				})
			}
		}
		result = append(result, contactGroupResponse{
			Avatar: value.Avatar, AvatarMembers: avatarMembers, ID: value.ID, Joined: value.Joined,
			MemberCount: value.MemberCount, Name: value.Name, Type: value.Type, Visibility: value.Visibility,
		})
	}
	return result
}

func legacyContactUsers(values []contactapp.User) []contactUserResponse {
	result := make([]contactUserResponse, 0, len(values))
	for _, value := range values {
		result = append(result, contactUserResponse{
			Avatar: value.Avatar, Email: value.Email, ID: value.ID, LastOnlineAt: formatOptionalTime(value.LastOnlineAt),
			Name: value.Name, Nickname: value.Nickname, Online: value.Online, Phone: value.Phone, Type: value.Type,
		})
	}
	return result
}
