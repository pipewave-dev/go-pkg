package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxRespBody = 1 << 20 // 1 MiB cap on callback responses

// Call modes, used as the "mode" metric label.
const (
	ModeSync   = "sync"
	ModeAsync  = "async"
	ModePubsub = "pubsub" // Class-2 events delivered over a pubsub broker instead of HTTP.
)

// CallObserver receives callback outcomes for instrumentation.
//
// Declared here, in the webhook package, so that webhook never imports the
// metrics package: the concrete implementation lives outside this module's
// webhook tree and is wired together in the server's main package. A nil
// observer disables observation, which is the default.
type CallObserver interface {
	// ObserveCall reports one completed HTTP attempt. statusCode is 0 when the
	// request never got a response.
	ObserveCall(eventType, mode string, dur time.Duration, statusCode int, err error)
	// ObserveRetry reports that an attempt is being retried.
	ObserveRetry(eventType, mode string)
	// ObserveDropped reports a callback abandoned after exhausting retries.
	ObserveDropped(eventType string)
}

type Sender struct {
	httpClient *http.Client
	signer     *Signer
	url        string
	obs        CallObserver
}

func NewSender(url string, signer *Signer) *Sender {
	return &Sender{
		httpClient: &http.Client{},
		signer:     signer,
		url:        url,
	}
}

// SetObserver attaches an observer. Safe to leave unset.
func (s *Sender) SetObserver(obs CallObserver) { s.obs = obs }

// Post delivers a signed callback using the sync mode label.
func (s *Sender) Post(ctx context.Context, eventType, callbackID string, data any, timeout time.Duration) (int, []byte, error) {
	return s.PostWithMode(ctx, eventType, callbackID, data, timeout, ModeSync)
}

// PostWithMode marshals data into the signed envelope and POSTs it to the
// callback URL, tagging the observation with mode. The per-call timeout
// bounds the whole request. Retries (if any) are the caller's job — pass the
// same callbackID so receivers can dedupe.
func (s *Sender) PostWithMode(ctx context.Context, eventType, callbackID string, data any, timeout time.Duration, mode string) (status int, body []byte, err error) {
	start := time.Now()
	defer func() {
		if s.obs != nil {
			s.obs.ObserveCall(eventType, mode, time.Since(start), status, err)
		}
	}()

	raw, err := json.Marshal(data)
	if err != nil {
		err = fmt.Errorf("webhook: marshal data for %s: %w", eventType, err)
		return status, body, err
	}
	envelope, err := json.Marshal(Body{
		Data: raw,
		Meta: Meta{
			SentAt:     time.Now().UnixMilli(),
			CallbackID: callbackID,
			EventType:  eventType,
		},
	})
	if err != nil {
		err = fmt.Errorf("webhook: marshal envelope for %s: %w", eventType, err)
		return status, body, err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(envelope))
	if err != nil {
		err = fmt.Errorf("webhook: build request for %s: %w", eventType, err)
		return status, body, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.signer != nil {
		req.Header.Set(SignatureHeader, s.signer.Sign(envelope))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("webhook: post %s: %w", eventType, err)
		return status, body, err
	}
	defer resp.Body.Close()
	status = resp.StatusCode

	body, err = io.ReadAll(io.LimitReader(resp.Body, maxRespBody))
	if err != nil {
		err = fmt.Errorf("webhook: read response for %s: %w", eventType, err)
		return status, nil, err
	}
	return status, body, err
}
