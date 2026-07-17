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

type Sender struct {
	httpClient *http.Client
	signer     *Signer
	url        string
}

func NewSender(url string, signer *Signer) *Sender {
	return &Sender{
		httpClient: &http.Client{},
		signer:     signer,
		url:        url,
	}
}

// Post marshals data into the signed envelope and POSTs it to the callback
// URL. The per-call timeout bounds the whole request. Retries (if any) are
// the caller's job — pass the same callbackID so receivers can dedupe.
func (s *Sender) Post(ctx context.Context, eventType, callbackID string, data any, timeout time.Duration) (int, []byte, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: marshal data for %s: %w", eventType, err)
	}
	body, err := json.Marshal(Body{
		Data: raw,
		Meta: Meta{
			SentAt:     time.Now().UnixMilli(),
			CallbackID: callbackID,
			EventType:  eventType,
		},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: marshal envelope for %s: %w", eventType, err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: build request for %s: %w", eventType, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, s.signer.Sign(body))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: post %s: %w", eventType, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBody))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("webhook: read response for %s: %w", eventType, err)
	}
	return resp.StatusCode, respBody, nil
}
