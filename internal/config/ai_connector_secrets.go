package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const (
	aiConnectorSecretVersion = 1
	aiConnectorSecretContext = "gofer/ai-connector-secret/v1"
)

func aiConnectorSecretAAD(userID, connectorID string) []byte {
	var payload []byte
	for _, field := range []string{aiConnectorSecretContext, userID, connectorID} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(field)))
		payload = append(payload, length[:]...)
		payload = append(payload, field...)
	}
	return payload
}

// EncryptAIConnectorSecret seals a connector signing secret so it can be
// stored in ai_connectors.secret_encrypted. The user and connector IDs are
// authenticated, so a copied ciphertext does not decrypt in another row.
func (s *AccountStore) EncryptAIConnectorSecret(userID, connectorID, plaintext string) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", nil
	}
	if userID == "" || connectorID == "" {
		return "", fmt.Errorf("connector secret requires user and connector")
	}
	nonce := make([]byte, s.aes.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate connector secret nonce: %w", err)
	}
	payload := []byte{aiConnectorSecretVersion}
	payload = append(payload, nonce...)
	payload = s.aes.Seal(payload, nonce, []byte(plaintext), aiConnectorSecretAAD(userID, connectorID))
	return base64.StdEncoding.EncodeToString(payload), nil
}

// DecryptAIConnectorSecret opens a value produced by
// EncryptAIConnectorSecret.
func (s *AccountStore) DecryptAIConnectorSecret(userID, connectorID, encrypted string) (string, error) {
	if strings.TrimSpace(encrypted) == "" {
		return "", nil
	}
	payload, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("decode connector secret: %w", err)
	}
	nonceSize := s.aes.NonceSize()
	if len(payload) < 1+nonceSize+s.aes.Overhead() || payload[0] != aiConnectorSecretVersion {
		return "", fmt.Errorf("stored connector secret has an unsupported format")
	}
	plaintext, err := s.aes.Open(nil, payload[1:1+nonceSize], payload[1+nonceSize:], aiConnectorSecretAAD(userID, connectorID))
	if err != nil {
		return "", fmt.Errorf("decrypt connector secret: %w", err)
	}
	return string(plaintext), nil
}
