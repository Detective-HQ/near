package middleware

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// minCompressBytes is the size below which gzip costs more than it saves.
const minCompressBytes = 1024

var gzipPool = sync.Pool{New: func() any {
	writer, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
	return writer
}}

var bufferPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// bufferedWriter holds the response until the handler returns so the
// compression decision can use the final size and content type.
type bufferedWriter struct {
	http.ResponseWriter
	status int
	buf    *bytes.Buffer
}

func (b *bufferedWriter) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.buf.Write(p)
}

// Gzip compresses JSON responses of at least 1 KB for clients that accept it.
// Activity pages and catalogues shrink by roughly 85-90%, which dominates
// transfer time on real networks. Writers and buffers are pooled.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		buf := bufferPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer bufferPool.Put(buf)

		bw := &bufferedWriter{ResponseWriter: w, buf: buf}
		next.ServeHTTP(bw, r)
		if bw.status == 0 {
			bw.status = http.StatusOK
		}

		header := w.Header()
		header.Add("Vary", "Accept-Encoding")
		contentType := header.Get("Content-Type")
		if buf.Len() < minCompressBytes || !strings.HasPrefix(contentType, "application/json") || header.Get("Content-Encoding") != "" {
			header.Set("Content-Length", strconv.Itoa(buf.Len()))
			w.WriteHeader(bw.status)
			_, _ = w.Write(buf.Bytes())
			return
		}

		header.Set("Content-Encoding", "gzip")
		header.Del("Content-Length")
		w.WriteHeader(bw.status)
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w)
		_, _ = gz.Write(buf.Bytes())
		_ = gz.Close()
		gzipPool.Put(gz)
	})
}
