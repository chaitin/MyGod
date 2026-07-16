package httpserver

const (
	imageMessageContentType    = "image/webp"
	maxImageMessageUploadBytes = 5 * 1024 * 1024
	maxImageMessageDimension   = 1920
	messageTypeImage           = "image"
)

type imageMessageBody struct {
	Type   string `json:"type"`
	FileID string `json:"file_id"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

func imageMessageSummary() string {
	return "[图片]"
}
