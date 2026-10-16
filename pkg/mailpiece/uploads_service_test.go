package mailpiece

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestUploadMakesABlobThenPutsItsBytesWithoutTheToken(t *testing.T) {
	var srv *sdkServer
	srv = newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		sdkReply(http.StatusOK, `{"signed_id":"SIGNED","attachable_sgid":"SGID","filename":"chart.png",
			"content_type":"image/png","byte_size":5,
			"direct_upload":{"url":"`+srv.URL+`/rails/active_storage/disk/KEY","headers":{"Content-Type":"image/png"}}}`)(w, r)
	})

	blob, err := NewUploadsService(sdkAccount(srv.URL)).Upload(context.Background(), "/tmp/charts/chart.png", []byte("hello"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if blob.AttachableSGID != "SGID" || blob.Filename != "chart.png" {
		t.Errorf("blob = %+v", blob)
	}

	reqs := srv.recorded()
	if len(reqs) != 2 {
		t.Fatalf("sent %d requests, want 2", len(reqs))
	}
	if reqs[0].Method != "POST" || reqs[0].URI != "/acme/rails/active_storage/direct_uploads.json" {
		t.Errorf("first request = %s %s", reqs[0].Method, reqs[0].URI)
	}
	// MD5 of "hello", base64.
	sdkAssertJSON(t, reqs[0].Body, `{"blob":{"filename":"chart.png","byte_size":5,"checksum":"XUFAKrxLKna5cZ2REBfFkg==","content_type":"image/png"}}`)

	put := reqs[1]
	if put.Method != "PUT" || put.URI != "/rails/active_storage/disk/KEY" || string(put.Body) != "hello" {
		t.Errorf("second request = %s %s %q", put.Method, put.URI, put.Body)
	}
	if put.Header.Get("Content-Type") != "image/png" {
		t.Errorf("the upload lost the blob's headers: %v", put.Header)
	}
	if put.Header.Get("Authorization") != "" {
		t.Error("the upload carried the token to the storage URL")
	}
}

func TestUploadRefusesAPlainHTTPStorageURL(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK,
		`{"attachable_sgid":"SGID","direct_upload":{"url":"http://storage.example.com/KEY","headers":{}}}`))

	_, err := NewUploadsService(sdkAccount(srv.URL)).Upload(context.Background(), "notes.txt", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "upload URL") {
		t.Fatalf("err = %v", err)
	}
	if n := len(srv.recorded()); n != 1 {
		t.Errorf("sent %d requests, want only the blob", n)
	}
}

func TestContentTypeOf(t *testing.T) {
	for name, want := range map[string]string{"a.PNG": "image/png", "b.pdf": "application/pdf", "noext": "text/plain"} {
		if got := contentTypeOf(name, []byte("plain words")); got != want {
			t.Errorf("contentTypeOf(%q) = %q, want %q", name, got, want)
		}
	}
}
