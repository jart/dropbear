// Package gcs provides a minimal Google Cloud Storage client using the JSON
// REST API. It supports resumable streaming uploads with automatic credential
// discovery (GCE metadata server in production, application default credentials
// locally).
package gcs

import (
	"dropbear/netty"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultBufSize = 4 * 1024 * 1024 // gcs requires multiple of 256kb
var errClosed = errors.New("gcs: writer already closed")

// Writer streams data to a GCS object via resumable upload.
// this class implements io.WriteCloser.
type Writer struct {
	bucket  string
	object  string
	session string
	offset  int64
	buf     []byte
	bufSize int
	err     error
}

// NewWriter creates a streaming upload to gs://bucket/object.
// Data is sent in chunks as Write is called. Call Close to finalize.
func NewWriter(bucket, object string) (*Writer, error) {
	token, err := gAuth.token()
	if err != nil {
		return nil, err
	}

	// initiate resumable upload
	var uri strings.Builder
	uri.WriteString("https://storage.googleapis.com/upload/storage/v1/b/")
	uri.WriteString(url.PathEscape(bucket))
	uri.WriteString("/o?uploadType=resumable")
	body := fmt.Sprintf(`{"name":%q}`, object)
	req, err := http.NewRequest("POST", uri.String(), strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := netty.GCSHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gcs initiate upload: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		// 403 forbidden probably means you need to give your vm gcs access
		return nil, fmt.Errorf("gcs initiate upload: %s", resp.Status)
	}
	session := resp.Header.Get("Location")
	if session == "" {
		return nil, errors.New("gcs: no session URI in response")
	}

	return &Writer{
		bucket:  bucket,
		object:  object,
		session: session,
		buf:     make([]byte, 0, defaultBufSize),
	}, nil
}

// Write buffers data and flushes in 256 KiB chunks.
func (w *Writer) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	written := 0
	for len(p) > 0 {
		n := copy(w.buf[len(w.buf):cap(w.buf)], p)
		w.buf = w.buf[:len(w.buf)+n]
		p = p[n:]
		written += n
		if len(w.buf) == cap(w.buf) {
			if err := w.flush(false); err != nil {
				w.err = err
				return written, err
			}
		}
	}
	return written, nil
}

// Close flushes remaining data and finalizes the upload.
func (w *Writer) Close() error {
	if w.err != nil {
		return w.err
	}
	err := w.flush(true)
	w.err = errClosed
	return err
}

func (w *Writer) flush(final bool) error {
	data := w.buf
	if len(data) == 0 && !final {
		return nil
	}

	token, err := gAuth.token()
	if err != nil {
		return fmt.Errorf("gcs token refresh: %w", err)
	}

	req, err := http.NewRequest("PUT", w.session, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	end := w.offset + int64(len(data)) - 1
	if final {
		total := w.offset + int64(len(data))
		if len(data) == 0 {
			req.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", total))
		} else {
			req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", w.offset, end, total))
		}
	} else {
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/*", w.offset, end))
	}

	resp, err := netty.GCSHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("gcs upload chunk: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// 308 = Resume Incomplete (more chunks expected)
	// 200/201 = upload complete
	if resp.StatusCode != 308 && resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("gcs upload chunk: %s", resp.Status)
	}

	w.offset += int64(len(data))
	w.buf = w.buf[:0]
	return nil
}
