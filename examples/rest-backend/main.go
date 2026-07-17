// Minimal callback receiver for pipewave-server. Verifies the Ed25519
// signature, answers the three sync events, and logs async events.
// Port this file to any language — it uses only the HTTP contract.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type meta struct {
	SentAt    int64  `json:"sent_at"`
	ID        string `json:"id"`
	EventType string `json:"event_type"`
}

type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta meta            `json:"meta"`
}

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	pubKeyB64 := flag.String("public-key", "", "pipewave webhook public key (base64); fetch from GET /api/v1/webhook/public-key")
	flag.Parse()

	var pubKey ed25519.PublicKey
	if *pubKeyB64 != "" {
		raw, err := base64.StdEncoding.DecodeString(*pubKeyB64)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			log.Fatalf("invalid public key: %v", err)
		}
		pubKey = raw
	}

	http.HandleFunc("POST /pipewave/callback", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		// 1. Verify signature (skipped when no key is configured — dev only).
		if pubKey != nil {
			sig, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Pipewave-Signature"))
			if err != nil || !ed25519.Verify(pubKey, body, sig) {
				http.Error(w, "bad signature", http.StatusUnauthorized)
				return
			}
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			http.Error(w, "bad envelope", http.StatusBadRequest)
			return
		}

		// 2. Reject stale deliveries (replay protection).
		if time.Since(time.UnixMilli(env.Meta.SentAt)) > 5*time.Minute {
			http.Error(w, "stale", http.StatusUnauthorized)
			return
		}

		log.Printf("event=%s id=%s data=%s", env.Meta.EventType, env.Meta.ID, env.Data)

		// 3. Switch on event type; sync events return a JSON body.
		switch env.Meta.EventType {
		case "inspect_token":
			var in struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal(env.Data, &in)
			// Demo auth: the token IS the user id.
			writeJSON(w, map[string]any{"user_id": in.Token, "is_anonymous": in.Token == "", "metadata": nil})

		case "handle_message":
			var in struct {
				InputType string `json:"input_type"`
				Data      []byte `json:"data"`
			}
			_ = json.Unmarshal(env.Data, &in)
			// Demo handler: echo everything back.
			writeJSON(w, map[string]any{"output_type": in.InputType + "_RESPONSE", "data": in.Data})

		case "on_new_connection":
			w.WriteHeader(http.StatusOK) // 2xx admits the connection

		default: // async events: acknowledge receipt
			w.WriteHeader(http.StatusOK)
		}
	})

	fmt.Println("rest-backend listening on", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
