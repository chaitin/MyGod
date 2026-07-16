package contact

type UserPresencePort interface {
	OnlineStatus([]string) map[string]bool
}

type AppPresencePort interface {
	IsOnline(string) bool
}
