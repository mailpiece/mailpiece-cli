package mailpiece

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 -- Active Storage's checksum is MD5
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// UploadsService uploads files the way the app's editor does, by Active Storage direct
// upload, for rich text to embed.
type UploadsService struct{ client *AccountClient }

// NewUploadsService builds an UploadsService bound to an account client.
func NewUploadsService(c *AccountClient) *UploadsService {
	return &UploadsService{client: c}
}

// Upload makes a blob for filename and puts data in it. The blob stays unattached until a
// rich text names it by its AttachableSGID; the app sweeps one left unattached.
func (s *UploadsService) Upload(ctx context.Context, filename string, data []byte) (*Blob, error) {
	sum := md5.Sum(data) // #nosec G401 -- Active Storage's checksum is MD5
	req := &directUploadRequest{Blob: directUploadBlob{
		Filename:    filepath.Base(filename),
		ByteSize:    int64(len(data)),
		Checksum:    base64.StdEncoding.EncodeToString(sum[:]),
		ContentType: contentTypeOf(filename, data),
	}}
	blob, _, err := send[Blob](ctx, s.client, "POST", req, "/rails/active_storage/direct_uploads.json")
	if err != nil {
		return nil, err
	}
	if blob.AttachableSGID == "" || blob.DirectUpload.URL == "" {
		return nil, ErrAPI(0, "The direct upload answered without an attachable_sgid or an upload URL")
	}
	if err := s.put(ctx, blob.DirectUpload, data); err != nil {
		return nil, err
	}
	return blob, nil
}

// put sends the bytes to the storage service's URL. It carries no token: the URL is signed,
// and can be on another host than the app.
func (s *UploadsService) put(ctx context.Context, part DirectPart, data []byte) error {
	if err := RequireSecureEndpoint(part.URL); err != nil {
		return ErrAPI(0, "The app answered an upload URL the file cannot go to: "+err.Error())
	}
	ctx = markProjectedRequest(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, part.URL, bytes.NewReader(data))
	if err != nil {
		return ErrUsage(fmt.Sprintf("The upload URL is not usable: %v", err))
	}
	for name, value := range part.Headers {
		req.Header.Set(name, value)
	}
	c := s.client.parent
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return networkError(err, "")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := limitedReadAll(resp.Body, 1024)
		return ErrAPI(resp.StatusCode, "The file was not stored: "+truncateString(strings.TrimSpace(string(body)), 200))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// contentTypeOf names a file's type by its extension, else by its first bytes.
func contentTypeOf(filename string, data []byte) string {
	if byExtension := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); byExtension != "" {
		if mediaType, _, err := mime.ParseMediaType(byExtension); err == nil {
			return mediaType
		}
	}
	mediaType, _, _ := mime.ParseMediaType(http.DetectContentType(data))
	return mediaType
}
