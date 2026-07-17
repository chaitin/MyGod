package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	fileapp "app/internal/application/file"
	"app/internal/media"
)

const (
	remoteMessageFetchTimeout     = 15 * time.Second
	remoteMessageFetchMaxRedirect = 3
	remoteImageWebPQuality        = 80
	maxAppInlineFileContentBytes  = 64 * 1024
	appInlineFileContentType      = "text/plain; charset=utf-8"
)

type downloadedRemoteMessageFile struct {
	Content     []byte
	ContentType string
	Filename    string
}

var validateRemoteMessageFetchURL = validateLinkFetchURL
var remoteMessageFetchHTTPClient = newRemoteMessageFetchHTTPClient()

func (s *Server) createRemoteImageMessageBody(ctx context.Context, rawURL string) (json.RawMessage, error) {
	remoteFile, err := downloadRemoteMessageFile(ctx, rawURL, maxRemoteMessageFileBytes)
	if err != nil {
		return nil, err
	}

	webpContent, width, height, err := media.ConvertImageToWebP(
		remoteFile.Content,
		maxImageMessageDimension,
		remoteImageWebPQuality,
	)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrImageEmpty):
			return nil, newAppRequestFailure("invalid_request", "图片不能为空")
		case errors.Is(err, media.ErrImageFormat):
			return nil, newAppRequestFailure("invalid_request", "图片格式不支持")
		default:
			return nil, newAppRequestFailure("invalid_request", "图片转换失败")
		}
	}
	if len(webpContent) > maxImageMessageUploadBytes {
		return nil, newAppRequestFailure("request_too_large", "图片不能超过 5MiB")
	}
	temporaryFile, err := s.saveRemoteMessageTemporaryFile(ctx, webpContent, imageMessageContentType)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(imageMessageBody{
		Type:   messageTypeImage,
		FileID: temporaryFile.ID,
		Width:  width,
		Height: height,
	})
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (s *Server) createRemoteFileMessageBody(ctx context.Context, rawURL string, rawName string) (json.RawMessage, string, error) {
	name, err := normalizeSpecifiedFileMessageName(rawName)
	if err != nil {
		return nil, "", newAppRequestFailure("invalid_request", err.Error())
	}
	remoteFile, err := downloadRemoteMessageFile(ctx, rawURL, maxRemoteMessageFileBytes)
	if err != nil {
		return nil, "", err
	}
	if len(remoteFile.Content) == 0 {
		return nil, "", newAppRequestFailure("invalid_request", "文件不能为空")
	}

	contentType := strings.TrimSpace(remoteFile.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	temporaryFile, err := s.saveRemoteMessageTemporaryFile(ctx, remoteFile.Content, contentType)
	if err != nil {
		return nil, "", err
	}
	body, err := json.Marshal(fileMessageBody{
		Type:      messageTypeFile,
		FileID:    temporaryFile.ID,
		Name:      name,
		SizeBytes: temporaryFile.SizeBytes,
	})
	if err != nil {
		return nil, "", err
	}

	return body, name, nil
}

func (s *Server) createInlineFileMessageBody(ctx context.Context, rawContent string, rawName string) (json.RawMessage, string, error) {
	name, err := normalizeSpecifiedFileMessageName(rawName)
	if err != nil {
		return nil, "", newAppRequestFailure("invalid_request", err.Error())
	}
	content := []byte(rawContent)
	if len(content) == 0 {
		return nil, "", newAppRequestFailure("invalid_request", "文件内容不能为空")
	}
	if len(content) > maxAppInlineFileContentBytes {
		return nil, "", newAppRequestFailure("request_too_large", "文件内容不能超过 64KiB")
	}

	temporaryFile, err := s.saveRemoteMessageTemporaryFile(ctx, content, appInlineFileContentType)
	if err != nil {
		return nil, "", err
	}
	body, err := json.Marshal(fileMessageBody{
		Type:      messageTypeFile,
		FileID:    temporaryFile.ID,
		Name:      name,
		SizeBytes: temporaryFile.SizeBytes,
	})
	if err != nil {
		return nil, "", err
	}

	return body, name, nil
}

func (s *Server) saveRemoteMessageTemporaryFile(ctx context.Context, content []byte, contentType string) (fileapp.TemporaryFile, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	temporaryFile, err := s.files.UploadTemporary(ctx, fileapp.UploadTemporaryCommand{
		Content:     bytes.NewReader(content),
		ContentType: contentType,
		SizeBytes:   int64(len(content)),
	})
	if err != nil {
		if fileapp.ErrorCodeOf(err) == fileapp.CodeStorageUnavailable {
			return fileapp.TemporaryFile{}, newAppRequestFailure("internal_error", "临时文件存储未配置")
		}
		return fileapp.TemporaryFile{}, newAppRequestFailure("internal_error", fileapp.ErrorMessage(err))
	}

	return temporaryFile, nil
}

func downloadRemoteMessageFile(ctx context.Context, rawURL string, maxBytes int64) (downloadedRemoteMessageFile, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	remoteURL, err := normalizeRemoteMessageURL(ctx, rawURL)
	if err != nil {
		return downloadedRemoteMessageFile{}, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, remoteMessageFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		return downloadedRemoteMessageFile{}, newAppRequestFailure("invalid_request", "URL 格式错误")
	}
	req.Header.Set("User-Agent", "MagicChat Remote Message Fetcher")

	resp, err := remoteMessageFetchHTTPClient.Do(req)
	if err != nil {
		return downloadedRemoteMessageFile{}, newAppRequestFailure("invalid_request", "下载 URL 失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return downloadedRemoteMessageFile{}, newAppRequestFailure("invalid_request", "下载 URL 失败")
	}
	if resp.ContentLength > maxBytes {
		return downloadedRemoteMessageFile{}, newAppRequestFailure("request_too_large", "文件不能超过 20MiB")
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return downloadedRemoteMessageFile{}, newAppRequestFailure("invalid_request", "读取 URL 内容失败")
	}
	if int64(len(content)) > maxBytes {
		return downloadedRemoteMessageFile{}, newAppRequestFailure("request_too_large", "文件不能超过 20MiB")
	}

	return downloadedRemoteMessageFile{
		Content:     content,
		ContentType: strings.TrimSpace(resp.Header.Get("Content-Type")),
		Filename:    remoteMessageFilename(resp.Header.Get("Content-Disposition"), remoteURL),
	}, nil
}

func normalizeRemoteMessageURL(ctx context.Context, rawURL string) (*url.URL, error) {
	messageURL := strings.TrimSpace(rawURL)
	if messageURL == "" {
		return nil, newAppRequestFailure("invalid_request", "URL 不能为空")
	}
	if strings.ContainsAny(messageURL, " \t\r\n") {
		return nil, newAppRequestFailure("invalid_request", "URL 格式错误")
	}

	parsedURL, err := url.Parse(messageURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, newAppRequestFailure("invalid_request", "URL 格式错误")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, newAppRequestFailure("invalid_request", "只支持 http 或 https URL")
	}
	if err := validateRemoteMessageFetchURL(ctx, parsedURL); err != nil {
		return nil, newAppRequestFailure("invalid_request", "URL 不允许访问")
	}

	return parsedURL, nil
}

func newRemoteMessageFetchHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: remoteMessageFetchTimeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		return dialLinkPreviewAddress(ctx, dialer, network, address)
	}

	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= remoteMessageFetchMaxRedirect {
				return errors.New("remote message redirect limit exceeded")
			}
			return validateRemoteMessageFetchURL(req.Context(), req.URL)
		},
		Timeout:   remoteMessageFetchTimeout,
		Transport: transport,
	}
}

func remoteMessageFilename(contentDisposition string, remoteURL *url.URL) string {
	if mediaType, params, err := mime.ParseMediaType(contentDisposition); err == nil && strings.EqualFold(mediaType, "attachment") {
		if filename := strings.TrimSpace(params["filename"]); filename != "" {
			return filename
		}
	}

	if remoteURL != nil {
		if filename := strings.TrimSpace(path.Base(strings.ReplaceAll(remoteURL.Path, "\\", "/"))); filename != "" && filename != "." && filename != "/" {
			return filename
		}
	}

	return "file"
}
