package webhook

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type PublicKeyVerifier struct {
	Alg               string `json:"alg"`
	PublicKeyInBase64 string `json:"public_key_in_base64"`
}

type Signer struct {
	priv ed25519.PrivateKey
}

// LoadOrGenerateSigner loads the Ed25519 seed (base64, one line) from
// keyFile, or generates a new key pair and persists the seed with 0600 perms.
func LoadOrGenerateSigner(keyFile string) (*Signer, error) {
	b, err := os.ReadFile(keyFile)
	if err == nil {
		seed, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if decErr != nil {
			return nil, fmt.Errorf("webhook: signing key file %s is not base64: %w", keyFile, decErr)
		}
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("webhook: signing key file %s: seed must be %d bytes, got %d", keyFile, ed25519.SeedSize, len(seed))
		}
		return &Signer{priv: ed25519.NewKeyFromSeed(seed)}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("webhook: read signing key file %s: %w", keyFile, err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("webhook: generate signing key: %w", err)
	}
	seedB64 := base64.StdEncoding.EncodeToString(priv.Seed())
	if err := os.WriteFile(keyFile, []byte(seedB64+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("webhook: persist signing key file %s: %w", keyFile, err)
	}
	return &Signer{priv: priv}, nil
}

func (s *Signer) Sign(body []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(s.priv, body))
}

func (s *Signer) Verify(body []byte, sigB64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	return ed25519.Verify(s.priv.Public().(ed25519.PublicKey), body, sig)
}

func (s *Signer) PublicKey() PublicKeyVerifier {
	return PublicKeyVerifier{
		Alg:               "Ed25519",
		PublicKeyInBase64: base64.StdEncoding.EncodeToString(s.priv.Public().(ed25519.PublicKey)),
	}
}
