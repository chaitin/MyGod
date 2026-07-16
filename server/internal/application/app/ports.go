package app

type ConnectionPort interface {
	IsOnline(string) bool
	CloseApp(string) int
}
