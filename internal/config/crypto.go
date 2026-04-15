// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

const (
	
	keySalt = "_ResultProxy_SafeVault_v1"
)





type CryptoService struct {
	key [32]byte
	keySource string
}


type secureEnvelope struct {
	IsSecure bool   `json:"_isSecure"`
	IV       string `json:"iv"`
	Data     string `json:"data"`
	AuthTag  string `json:"authTag"`
}

func (cs *CryptoService) KeySource() string {
	return cs.keySource
}

func NewCryptoService(userDataPath string) (*CryptoService, error) {
	machineID, source, err := getHardwareID(userDataPath)
	if err != nil {
		return nil, fmt.Errorf("getting hardware ID: %w", err)
	}
	cs := NewCryptoServiceWithID(machineID)
	cs.keySource = source
	return cs, nil
}

func StableHardwareID(userDataPath string) (string, error) {
	machineID, _, err := getHardwareID(userDataPath)
	if err != nil {
		return "", err
	}
	return machineID, nil
}



func NewCryptoServiceWithID(machineID string) *CryptoService {
	h := sha256.Sum256([]byte(machineID + keySalt))
	return &CryptoService{key: h}
}



func (cs *CryptoService) Encrypt(data any) (string, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshaling data: %w", err)
	}

	block, err := aes.NewCipher(cs.key[:])
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	
	
	
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generating IV: %w", err)
	}

	
	gcmWithNonce, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		return "", fmt.Errorf("creating GCM with nonce size 16: %w", err)
	}

	
	sealed := gcmWithNonce.Seal(nil, iv, plaintext, nil)

	
	ciphertext := sealed[:len(sealed)-gcmWithNonce.Overhead()]
	authTag := sealed[len(sealed)-gcmWithNonce.Overhead():]

	env := secureEnvelope{
		IsSecure: true,
		IV:       base64.StdEncoding.EncodeToString(iv),
		Data:     base64.StdEncoding.EncodeToString(ciphertext),
		AuthTag:  base64.StdEncoding.EncodeToString(authTag),
	}

	result, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling envelope: %w", err)
	}
	return string(result), nil
}




func (cs *CryptoService) Decrypt(rawStr string) (json.RawMessage, error) {
	var env secureEnvelope
	if err := json.Unmarshal([]byte(rawStr), &env); err != nil {
		return nil, fmt.Errorf("parsing envelope: %w", err)
	}

	
	if !env.IsSecure || env.Data == "" || env.IV == "" || env.AuthTag == "" {
		return json.RawMessage(rawStr), nil
	}

	iv, err := base64.StdEncoding.DecodeString(env.IV)
	if err != nil {
		return nil, fmt.Errorf("decoding IV: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, fmt.Errorf("decoding data: %w", err)
	}

	authTag, err := base64.StdEncoding.DecodeString(env.AuthTag)
	if err != nil {
		return nil, fmt.Errorf("decoding authTag: %w", err)
	}

	block, err := aes.NewCipher(cs.key[:])
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return nil, fmt.Errorf("creating GCM with nonce size %d: %w", len(iv), err)
	}

	
	sealed := append(ciphertext, authTag...)

	plaintext, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return json.RawMessage(plaintext), nil
}


func (cs *CryptoService) DecryptInto(rawStr string, dst any) error {
	raw, err := cs.Decrypt(rawStr)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}



var hardwareIDRegex = regexp.MustCompile(`[a-fA-F0-9\-]{8,}`)


func getHardwareID(userDataPath string) (string, string, error) {
	var id string
	var err error

	switch runtime.GOOS {
	case "windows":
		id, err = windowsMachineGUID()
	case "darwin":
		id, err = darwinPlatformUUID()
	default:
		id, err = linuxMachineID()
	}

	if err == nil && id != "" {
		return id, "hardware", nil
	}

	fb, fbErr := getOrCreateFallbackID(userDataPath)
	if fbErr != nil {
		if err != nil {
			return "", "", fmt.Errorf("hardware id failed: %v; fallback id failed: %w", err, fbErr)
		}
		return "", "", fbErr
	}
	return fb, "fallback", nil
}

func darwinPlatformUUID() (string, error) {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", fmt.Errorf("reading IOPlatformUUID: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "IOPlatformUUID") {
			match := hardwareIDRegex.FindString(line)
			if match != "" {
				return match, nil
			}
		}
	}
	return "", errors.New("IOPlatformUUID not found")
}

func linuxMachineID() (string, error) {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", fmt.Errorf("reading /etc/machine-id: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if len(id) < 8 {
		return "", errors.New("machine-id too short")
	}
	return id, nil
}

func getOrCreateFallbackID(userDataPath string) (string, error) {
	fallbackPath := filepath.Join(userDataPath, ".machine-fallback-id")

	
	if data, err := os.ReadFile(fallbackPath); err == nil {
		stored := strings.TrimSpace(string(data))
		if len(stored) >= 32 {
			return stored, nil
		}
	}

	
	newID := uuid.New().String()

	dir := filepath.Dir(fallbackPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating fallback id dir: %w", err)
	}
	if err := os.WriteFile(fallbackPath, []byte(newID), 0o600); err != nil {
		return "", fmt.Errorf("writing fallback id: %w", err)
	}

	return newID, nil
}
