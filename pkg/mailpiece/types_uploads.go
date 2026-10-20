package mailpiece

// Blob is what POST /rails/active_storage/direct_uploads.json answers: the blob made for a
// file, with where to PUT its bytes. AttachableSGID is what an <action-text-attachment>
// names it by.
type Blob struct {
	Key            string     `json:"key"`
	Filename       string     `json:"filename"`
	ContentType    string     `json:"content_type"`
	ByteSize       int64      `json:"byte_size"`
	Checksum       string     `json:"checksum"`
	SignedID       string     `json:"signed_id"`
	AttachableSGID string     `json:"attachable_sgid"`
	DirectUpload   DirectPart `json:"direct_upload"`
}

// DirectPart is where a blob's bytes go and the headers they go with.
type DirectPart struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// directUploadRequest is the body of POST /rails/active_storage/direct_uploads.json.
type directUploadRequest struct {
	Blob directUploadBlob `json:"blob"`
}

type directUploadBlob struct {
	Filename    string `json:"filename"`
	ByteSize    int64  `json:"byte_size"`
	Checksum    string `json:"checksum"`
	ContentType string `json:"content_type"`
}
