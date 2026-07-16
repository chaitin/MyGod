package httpserver

import (
	"errors"
	"path"
	"strings"
)

const (
	maxFileMessageNameLength = 255
	messageTypeFile          = "file"
)

type fileMessageBody struct {
	Type      string `json:"type"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

func normalizeClientMessageID(rawClientMessageID string) (string, error) {
	clientMessageID := strings.TrimSpace(rawClientMessageID)
	if clientMessageID == "" {
		return "", errors.New("客户端消息 ID 不能为空")
	}
	if len([]rune(clientMessageID)) > maxClientMessageIDLength {
		return "", errors.New("客户端消息 ID 不能超过 128 个字符")
	}
	return clientMessageID, nil
}

func normalizeFileMessageName(rawName string) (string, error) {
	name := strings.TrimSpace(path.Base(strings.ReplaceAll(rawName, "\\", "/")))
	if name == "" || name == "." || name == "/" {
		return "", errors.New("文件名不能为空")
	}
	if len([]rune(name)) > maxFileMessageNameLength {
		return "", errors.New("文件名不能超过 255 个字符")
	}
	return name, nil
}

func normalizeSpecifiedFileMessageName(rawName string) (string, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return "", errors.New("文件名不能为空")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("文件名不能包含路径")
	}
	if len([]rune(name)) > maxFileMessageNameLength {
		return "", errors.New("文件名不能超过 255 个字符")
	}
	return name, nil
}

func fileMessageSummary(name string) string {
	return "[文件] " + strings.TrimSpace(name)
}
