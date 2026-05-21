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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// keySalt is the legacy public constant used by builds prior to the
	// per-install random salt. We still derive a legacy key on startup so
	// existing configs can be migrated and re-encrypted with the new key
	// without losing user data.
	keySalt  = "_ResultProxy_SafeVault_v1"
	hwidSalt = "_ResultProxy_Sub_HWID_v1"

	// installSaltFile lives alongside the config under the user's app data
	// directory. 32 random bytes, base64-encoded, mode 0o600. Without this
	// file, an attacker who can read /etc/machine-id (Linux, world-readable
	// by default) or Win32_ComputerSystemProduct UUID (Windows, readable to
	// any local user) still cannot reconstruct the AES key — the salt is
	// user-private inside %APPDATA%.
	installSaltFile  = ".install-salt"
	installSaltBytes = 32

	// pbkdf2EncryptIterations is the cost factor for deriving the config
	// encryption key from machineID + installSalt. Lower than the export KDF
	// (which runs on user-typed passwords) because:
	//   - runs on every app launch — too-slow KDF means slow cold start.
	//   - input (machineID + 32B random salt) has far more entropy than a
	//     human password, so brute force is already infeasible.
	// 200k SHA-256 iterations is ~70-150ms on a modern CPU.
	pbkdf2EncryptIterations = 200_000
)


type CryptoService struct {
	key       [32]byte
	legacyKey [32]byte
	keySource string

	// reencryptMu protects needsReencrypt. The flag is set when Decrypt
	// falls back to legacyKey — config.Manager observes it after the next
	// load and re-saves the data encrypted under the new key, then clears
	// the flag.
	reencryptMu    sync.Mutex
	needsReencrypt bool
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

// NewCryptoService builds the live encryption service for the running app.
// It derives two keys:
//
//   - key (PBKDF2(machineID, installSalt, 200k iterations)) — the current
//     scheme, used to encrypt all new writes.
//   - legacyKey (sha256(machineID + keySalt)) — the pre-salt scheme, used
//     only to decrypt configs written by older builds during migration.
//
// On first run the install-salt file is generated (32 random bytes, mode
// 0o600). On subsequent runs it is read back from disk. If the salt file is
// lost (e.g. user copied only proxy_config.json to a new install) decryption
// falls back to legacyKey automatically.
func NewCryptoService(userDataPath string) (*CryptoService, error) {
	machineID, source, err := getHardwareID(userDataPath)
	if err != nil {
		return nil, fmt.Errorf("getting hardware ID: %w", err)
	}
	salt, err := loadOrCreateInstallSalt(userDataPath)
	if err != nil {
		return nil, fmt.Errorf("install salt: %w", err)
	}
	derived := pbkdf2.Key([]byte(machineID), salt, pbkdf2EncryptIterations, 32, sha256.New)
	legacy := sha256.Sum256([]byte(machineID + keySalt))

	cs := &CryptoService{keySource: source, legacyKey: legacy}
	copy(cs.key[:], derived)
	return cs, nil
}

// loadOrCreateInstallSalt reads .install-salt from userDataPath. When the
// file is missing it generates fresh random bytes and persists them. The
// directory is created with 0o700 and the file with 0o600 so other local
// users cannot read the salt — that's the entire defence over the legacy
// keySalt constant.
func loadOrCreateInstallSalt(userDataPath string) ([]byte, error) {
	if userDataPath == "" {
		return nil, errors.New("install salt: empty user data path")
	}
	path := filepath.Join(userDataPath, installSaltFile)
	if data, err := os.ReadFile(path); err == nil {
		decoded, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if decErr == nil && len(decoded) >= 16 {
			return decoded, nil
		}
		// Corrupted or truncated salt file: fall through to regenerate.
		// Existing configs will be migrated via the legacy-key path because
		// the new salt produces a new key.
	}
	salt := make([]byte, installSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating install salt: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(salt)
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("writing install salt: %w", err)
	}
	return salt, nil
}

// NeedsReencrypt reports whether the most recent Decrypt fell back to the
// legacy key. config.Manager uses this after loadLocked to trigger a
// re-save with the current key, then calls ClearReencryptFlag.
func (cs *CryptoService) NeedsReencrypt() bool {
	cs.reencryptMu.Lock()
	defer cs.reencryptMu.Unlock()
	return cs.needsReencrypt
}

// ClearReencryptFlag must be called by config.Manager after a successful
// re-save under the new key. Without this, every subsequent Decrypt of the
// (already re-encrypted) config would still report needsReencrypt=true.
func (cs *CryptoService) ClearReencryptFlag() {
	cs.reencryptMu.Lock()
	cs.needsReencrypt = false
	cs.reencryptMu.Unlock()
}

func (cs *CryptoService) setNeedsReencrypt() {
	cs.reencryptMu.Lock()
	cs.needsReencrypt = true
	cs.reencryptMu.Unlock()
}

func StableHardwareID(userDataPath string) (string, error) {
	var hwidSource string

	if runtime.GOOS == "windows" {
		if wmiUUID, err := getWindowsWMIUUID(); err == nil && wmiUUID != "" {
			hwidSource = wmiUUID
		}
	}

	if hwidSource == "" {
		machineID, _, err := getHardwareID(userDataPath)
		if err != nil {
			return "", err
		}
		hwidSource = machineID
	}

	h := sha256.Sum256([]byte(hwidSource + hwidSalt))
	return hex.EncodeToString(h[:]), nil
}



// NewCryptoServiceWithID builds a CryptoService whose primary key follows
// the legacy (pre-salt) scheme. Retained for tests and any consumer that
// needs deterministic encryption from a machine ID without disk I/O.
// Production code uses NewCryptoService instead.
func NewCryptoServiceWithID(machineID string) *CryptoService {
	h := sha256.Sum256([]byte(machineID + keySalt))
	cs := &CryptoService{key: h, legacyKey: h}
	return cs
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

	sealed := append(ciphertext, authTag...)

	// Try the current key first. If GCM rejects the ciphertext we fall back
	// to the legacy key — this is how a config encrypted by an older build
	// (sha256(machineID + keySalt), no per-install salt) is opened on the
	// first run of a build that introduced the install-salt scheme. A
	// successful legacy decrypt sets needsReencrypt so config.Manager can
	// rewrite the file under the new key.
	if plaintext, err := openWithKey(cs.key[:], iv, sealed); err == nil {
		return json.RawMessage(plaintext), nil
	}
	// Same-key short-circuit: NewCryptoServiceWithID sets key == legacyKey
	// and the second attempt would just repeat the first failure. Skip it.
	if cs.legacyKey != cs.key {
		if plaintext, err := openWithKey(cs.legacyKey[:], iv, sealed); err == nil {
			cs.setNeedsReencrypt()
			return json.RawMessage(plaintext), nil
		}
	}
	return nil, fmt.Errorf("decrypting: tag mismatch with both current and legacy keys")
}

// openWithKey runs AES-GCM-Open with the supplied key and 16-byte nonce.
// Extracted so Decrypt can try multiple keys cheaply (only the cipher init
// is repeated; the parsing/base64 work is shared).
func openWithKey(key, nonce, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(nonce))
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, sealed, nil)
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
