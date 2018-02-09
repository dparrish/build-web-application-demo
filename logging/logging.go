package logging

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	wrap "gopkg.in/go-on/wrap.v2"
)

type RequestLog struct {
	Timestamp      civil.DateTime `bigquery:"timestamp"`
	URI            string         `bigquery:"uri"`
	Method         string         `bigquery:"method"`
	Proto          string         `bigquery:"proto"`
	Host           string         `bigquery:"source"`
	RequestHeader  []string       `bigquery:"request_header"`
	RequestLength  int            `bigquery:"request_length"`
	RequestHash    string         `bigquery:"request_hash"`
	ResponseCode   int            `bigquery:"response_code"`
	ResponseHeader []string       `bigquery:"response_header"`
}

type LogMiddleware struct {
	Table *bigquery.Table
}

func (m *LogMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Read the body, then re-inject it back into the request for the next middleware.
		body, _ := ioutil.ReadAll(req.Body)
		req.Body.Close()
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		entry := &RequestLog{
			Timestamp:     civil.DateTimeOf(time.Now()),
			URI:           req.RequestURI,
			Method:        req.Method,
			Proto:         req.Proto,
			Host:          req.Host,
			RequestLength: len(body),
		}

		for k, values := range req.Header {
			for _, v := range values {
				entry.RequestHeader = append(entry.RequestHeader, fmt.Sprintf("%s=%s", k, v))
			}
		}

		if len(body) > 0 {
			// Don't store the entire request body, just a SHA-1 hash of it.
			hash := sha1.New()
			hash.Write(body)
			entry.RequestHash = base64.URLEncoding.EncodeToString(hash.Sum(nil))
		}

		peek := wrap.NewPeek(w, func(p *wrap.Peek) bool {
			p.FlushMissing()
			return true
		})
		next.ServeHTTP(peek, req)

		entry.ResponseCode = peek.Code
		if entry.ResponseCode == 0 {
			// If the handler doesn't explicitly set a response code, 200 is assumed.
			entry.ResponseCode = http.StatusOK
		}
		for k, values := range peek.ResponseWriter.Header() {
			for _, v := range values {
				entry.ResponseHeader = append(entry.ResponseHeader, fmt.Sprintf("%s=%s", k, v))
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		u := m.Table.Uploader()
		// Write the log entry to BigQuery.
		if err := u.Put(ctx, entry); err != nil {
			log.Printf("Error writing access log to BigQuery: %v", err)
			if m, ok := err.(bigquery.PutMultiError); ok {
				for _, err := range m {
					log.Print(err)
				}
			}
		}
	})
}
