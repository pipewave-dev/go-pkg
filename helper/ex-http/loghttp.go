package exhttp

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// https://stackoverflow.com/questions/39527847/is-there-middleware-for-go-http-client

// This type implements the http.RoundTripper interface.
type loggingRoundTripper struct {
	proxied http.RoundTripper
	cf      LoggingConfig
}
type bodyFormat int

const (
	JsonFormat bodyFormat = iota + 1
	RawStringFormat
)

type LoggingConfig struct {
	ShowHeaders bool

	ShowBody            bool
	FieldValueMaxLength int
	// LogStoreUpload .. upload to store if body too long (can't log to stdout)
	// Should be upload in background and return Upload Key Path instantly for increase performance.
	// If there is any error when upload. Handle and notify in other way.
	BodyFormatForLog bodyFormat
}

func NewLoggingRoundTripper(base http.RoundTripper, cf LoggingConfig) http.RoundTripper {
	return loggingRoundTripper{
		proxied: base,
		cf:      cf,
	}
}

func (lrt loggingRoundTripper) RoundTrip(req *http.Request) (res *http.Response, e error) {
	ctx := req.Context()
	level := slog.LevelInfo

	startTime := time.Now()
	logAttrs := []slog.Attr{}
	reqLogAttrs := []slog.Attr{}
	resLogAttrs := []slog.Attr{}

	if lrt.cf.ShowHeaders {
		reqLogAttrs = append(reqLogAttrs, slog.Any("headers", req.Header))
	}
	defer func() {
		apiWaitDuration := time.Since(startTime)

		logAttrs = append(logAttrs,
			slog.String("url", req.URL.String()),
			slog.String("method", req.Method),
			slog.String("process_duration", apiWaitDuration.String()),
			slog.Any("request", reqLogAttrs),
			slog.Any("response", resLogAttrs),
		)

		slog.LogAttrs(ctx, level, "http_log", logAttrs...)
	}()

	if lrt.cf.ShowBody && req.Body != nil {
		req.Body = lrt.logLongBody(req.Body, lrt.cf.BodyFormatForLog, &reqLogAttrs, "body")
	}

	// Send the request, get the response (or the error)
	res, e = lrt.proxied.RoundTrip(req)

	// Handle the result.
	if e != nil {
		logAttrs = append(logAttrs,
			slog.Any("err", e))
		return res, e
	}
	if res.StatusCode >= http.StatusBadRequest {
		level = slog.LevelWarn
	}

	logAttrs = append(logAttrs, slog.Int("status", res.StatusCode))
	if lrt.cf.ShowHeaders {
		resLogAttrs = append(resLogAttrs, slog.Any("headers", res.Header))
	}
	if lrt.cf.ShowBody && res.Body != nil {
		res.Body = lrt.logLongBody(res.Body, lrt.cf.BodyFormatForLog, &resLogAttrs, "body")
	}
	return res, nil
}

func (lrt loggingRoundTripper) logLongBody(
	body io.ReadCloser,
	format bodyFormat,
	logAttrs *[]slog.Attr,
	logField string,
) io.ReadCloser {
	maxLengthJSON := lrt.cf.FieldValueMaxLength
	var reqb bytes.Buffer

	tee := io.TeeReader(body, &reqb)
	reader1, _ := io.ReadAll(tee)

	if len(reader1) < maxLengthJSON {
		switch format {
		case JsonFormat:
			*logAttrs = append(*logAttrs, slog.Any(logField, string(reader1)))
		case RawStringFormat:
			*logAttrs = append(*logAttrs, slog.Any(logField, string(reader1)))
		}
		return io.NopCloser(&reqb)
	}

	switch format {
	case JsonFormat:
		*logAttrs = append(*logAttrs, slog.Any(logField, UploadJson(reader1)))
	default:
		*logAttrs = append(*logAttrs, slog.Any(logField, UploadTxt(reader1)))
	}
	return io.NopCloser(&reqb)
}

// Interface for slogger Uploader
type UploadTxt []byte

func (x UploadTxt) Content() []byte {
	return x
}
func (x UploadTxt) Ext() string { return "txt" }

type UploadJson []byte

func (x UploadJson) Content() []byte {
	return x
}
func (x UploadJson) Ext() string { return "json" }
