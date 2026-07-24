# Deterministic CA Regeneration + Stale-Entry Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make app reinstall transparent by regenerating a byte-identical root CA from a stable device seed, and surface any leftover duplicate CA entries in the system trust store.

**Architecture:** The Go CA generator gains a deterministic path: given a non-empty `seed`, it derives all randomness from a ChaCha20 keystream keyed by `SHA-256(salt || seed)`, and fixes the certificate serial and validity dates, so the same seed always yields the identical cert+key bytes. The Android side passes `Settings.Secure.ANDROID_ID` (which survives reinstall for the same signing key) as the seed via a new `Mobile.setFilterCASeed` call, and `CertStore` gains a stale-duplicate counter that the wizard/settings surface as a banner.

**Tech Stack:** Go (`crypto/x509`, `golang.org/x/crypto/chacha20`), gomobile bind, Kotlin/Android (`java.security.KeyStore` "AndroidCAStore"), Jetpack Compose.

## Global Constraints

- Go CA subject CommonName is exactly `ResultV AdBlock Root CA` (`internal/filter/ca/ca.go:26`, constant `caCN`) — do not change it; the Android stale-entry match depends on it.
- CA files remain `ca.crt` (PEM cert) and `ca.key` (PEM PKCS#1 RSA key) under `dataDir/filter/`.
- Deterministic branch is a **fallback**: when `ca.crt` already exists on disk, load it unchanged (seed ignored). Never regenerate over an existing file.
- Empty seed (`""`) → keep the existing random generation path unchanged.
- RSA key size stays 2048 bits.
- DRBG byte-consumption order is a fixed contract: RSA key first, then 16 bytes for the serial. Do not reorder.
- Android min API for this code path is 29 (browser ad-block requirement).
- New user-facing strings go in `android/app/src/main/res/values/strings.xml` and every locale variant that already exists alongside it.
- Windows dev host: run Gradle via `.\gradlew.bat`, Go via `go` on PATH.

---

### Task 1: Deterministic DRBG + CA generation (Go)

**Files:**
- Create: `internal/filter/ca/drbg.go`
- Modify: `internal/filter/ca/ca.go` (signature of `EnsureRoot`, new `generateDeterministic`, fixed-date constants)
- Modify: `internal/filter/manager.go:438-444` (caller passes `""` for now to keep build green)
- Test: `internal/filter/ca/ca_test.go` (new file)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces:
  - `func EnsureRoot(filterDir, seed string) (*Root, error)` — seed `""` = random path (unchanged behavior); non-empty = deterministic.
  - `func newDRBG(seed string) io.Reader` (in `drbg.go`) — unbounded deterministic keystream.

- [ ] **Step 1: Write the failing test**

Create `internal/filter/ca/ca_test.go`:

```go
package ca

import (
	"bytes"
	"os"
	"testing"
)

func TestEnsureRoot_DeterministicSameSeed(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	seed := "device-android-id-abc123"

	r1, err := EnsureRoot(dir1, seed)
	if err != nil {
		t.Fatalf("EnsureRoot dir1: %v", err)
	}
	r2, err := EnsureRoot(dir2, seed)
	if err != nil {
		t.Fatalf("EnsureRoot dir2: %v", err)
	}

	cert1, _ := os.ReadFile(r1.CertificatePath)
	cert2, _ := os.ReadFile(r2.CertificatePath)
	if !bytes.Equal(cert1, cert2) {
		t.Fatal("same seed produced different certificate bytes")
	}
	key1, _ := os.ReadFile(r1.PrivateKeyPath)
	key2, _ := os.ReadFile(r2.PrivateKeyPath)
	if !bytes.Equal(key1, key2) {
		t.Fatal("same seed produced different key bytes")
	}
}

func TestEnsureRoot_DifferentSeedDiffers(t *testing.T) {
	a, err := EnsureRoot(t.TempDir(), "seed-A")
	if err != nil {
		t.Fatalf("EnsureRoot A: %v", err)
	}
	b, err := EnsureRoot(t.TempDir(), "seed-B")
	if err != nil {
		t.Fatalf("EnsureRoot B: %v", err)
	}
	ca, _ := os.ReadFile(a.CertificatePath)
	cb, _ := os.ReadFile(b.CertificatePath)
	if bytes.Equal(ca, cb) {
		t.Fatal("different seeds produced identical certificates")
	}
}

func TestEnsureRoot_EmptySeedStillValid(t *testing.T) {
	r, err := EnsureRoot(t.TempDir(), "")
	if err != nil {
		t.Fatalf("EnsureRoot empty seed: %v", err)
	}
	if r.Certificate == nil || r.PrivateKey == nil {
		t.Fatal("empty seed must still yield a usable CA")
	}
	if r.Certificate.Subject.CommonName != caCN {
		t.Fatalf("unexpected CN %q", r.Certificate.Subject.CommonName)
	}
}

func TestEnsureRoot_ExistingFilesReloadedIgnoringSeed(t *testing.T) {
	dir := t.TempDir()
	first, err := EnsureRoot(dir, "seed-one")
	if err != nil {
		t.Fatalf("first EnsureRoot: %v", err)
	}
	firstCert, _ := os.ReadFile(first.CertificatePath)
	// Different seed, but files already exist -> must reload, not regenerate.
	second, err := EnsureRoot(dir, "seed-two")
	if err != nil {
		t.Fatalf("second EnsureRoot: %v", err)
	}
	secondCert, _ := os.ReadFile(second.CertificatePath)
	if !bytes.Equal(firstCert, secondCert) {
		t.Fatal("existing CA was regenerated instead of reloaded")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/filter/ca/`
Expected: FAIL — compile error `too few arguments in call to EnsureRoot` (signature is still one-arg).

- [ ] **Step 3: Create the DRBG**

Create `internal/filter/ca/drbg.go`:

```go
package ca

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/chacha20"
)

// deterministicReader yields an unbounded, reproducible keystream. Keyed by
// SHA-256(drbgSalt || seed) and run as ChaCha20 over an all-zero plaintext,
// it gives the same bytes every run for a given seed — enough to drive RSA
// key generation, which HKDF's 255*hashlen output cap could not guarantee.
type deterministicReader struct {
	cipher *chacha20.Cipher
}

// drbgSalt namespaces this stream. Bump the version suffix only with a
// deliberate, breaking change to CA reproduction.
const drbgSalt = "resultv-ca-drbg-v1"

func newDRBG(seed string) io.Reader {
	sum := sha256.Sum256(append([]byte(drbgSalt), seed...))
	// ChaCha20 needs a 32-byte key and 12-byte nonce; a zero nonce is fine
	// because each seed produces a distinct key.
	c, err := chacha20.NewUnauthenticatedCipher(sum[:], make([]byte, chacha20.NonceSize))
	if err != nil {
		// NewUnauthenticatedCipher only errors on wrong key/nonce sizes,
		// which are fixed constants here.
		panic(err)
	}
	return &deterministicReader{cipher: c}
}

func (r *deterministicReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	r.cipher.XORKeyStream(p, p)
	return len(p), nil
}
```

- [ ] **Step 4: Rewrite the generator to take a seed**

In `internal/filter/ca/ca.go`, add fixed-date constants near the existing `const` block (after line 27):

```go
// Fixed validity window for deterministically generated roots. Using
// time.Now() would make the certificate bytes non-reproducible, defeating
// the whole point of regenerating the same CA after reinstall.
var (
	deterministicNotBefore = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	deterministicNotAfter  = time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
)
```

Change `EnsureRoot` (currently lines 38-48) to:

```go
// EnsureRoot creates or loads the root CA under filterDir. When the CA does
// not yet exist and seed is non-empty, generation is deterministic: the same
// seed reproduces byte-identical cert and key, so a reinstalled app recreates
// the exact CA already trusted by the system. An empty seed falls back to
// random generation.
func EnsureRoot(filterDir, seed string) (*Root, error) {
	if err := os.MkdirAll(filterDir, 0o700); err != nil {
		return nil, err
	}
	certPath := filepath.Join(filterDir, certFile)
	keyPath := filepath.Join(filterDir, keyFile)
	if _, err := os.Stat(certPath); err == nil {
		return loadRoot(certPath, keyPath)
	}
	if seed != "" {
		return generateDeterministic(certPath, keyPath, seed)
	}
	return generateRoot(certPath, keyPath)
}
```

Add `generateDeterministic` immediately after `generateRoot` (after current line 94). It mirrors `generateRoot` but sources all randomness from the DRBG and uses fixed serial/dates:

```go
func generateDeterministic(certPath, keyPath, seed string) (*Root, error) {
	drbg := newDRBG(seed)
	key, err := rsa.GenerateKey(drbg, 2048)
	if err != nil {
		return nil, err
	}
	// Serial from the same stream, consumed AFTER the key. This order is a
	// fixed contract — changing it breaks reproduction of existing CAs.
	serialBytes := make([]byte, 16)
	if _, err := io.ReadFull(drbg, serialBytes); err != nil {
		return nil, err
	}
	serial := new(big.Int).SetBytes(serialBytes)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caCN,
			Organization: []string{"ResultV"},
		},
		NotBefore:             deterministicNotBefore,
		NotAfter:              deterministicNotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	// RSA PKCS#1 v1.5 signing ignores the rand argument, so passing the DRBG
	// keeps the call deterministic without consuming more stream bytes.
	der, err := x509.CreateCertificate(drbg, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return writeRoot(certPath, keyPath, der, key)
}
```

Refactor the file-writing tail of `generateRoot` into a shared `writeRoot` so both paths stay DRY. Replace `generateRoot`'s body from the `certPEM := ...` line (current line 75) through its `return` with:

```go
	return writeRoot(certPath, keyPath, der, key)
}

// writeRoot persists cert (DER) and key to disk and returns the parsed Root.
func writeRoot(certPath, keyPath string, der []byte, key *rsa.PrivateKey) (*Root, error) {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Root{
		CertificatePath: certPath,
		PrivateKeyPath:  keyPath,
		Certificate:     cert,
		PrivateKey:      key,
	}, nil
}
```

Add `"io"` to the import block in `ca.go` (alongside the existing imports at lines 10-21). `math/big`, `crypto/x509`, `crypto/x509/pkix`, `encoding/pem`, `time` are already imported.

- [ ] **Step 5: Keep the existing caller compiling**

In `internal/filter/manager.go`, update `CARootPath` (line 439) from `ca.EnsureRoot(m.FilterDir())` to:

```go
	root, err := ca.EnsureRoot(m.FilterDir(), "")
```

(Task 2 replaces `""` with the real seed. This keeps the build and existing behavior green now.)

- [ ] **Step 6: Add x/crypto/chacha20 as a direct dependency**

Run: `go mod tidy`
Expected: `golang.org/x/crypto` moves from `// indirect` to a direct require in `go.mod`; `go.sum` unchanged otherwise.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/filter/ca/ ./internal/filter/`
Expected: PASS — including the pre-existing `TestManager_CARootPath_ReturnsExistingFile`.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/ca/ca.go internal/filter/ca/drbg.go internal/filter/ca/ca_test.go internal/filter/manager.go go.mod go.sum
git commit -m "feat(ca): deterministic root CA regeneration from a stable seed"
```

---

### Task 2: Thread the seed through the manager and mobile API (Go)

**Files:**
- Modify: `internal/filter/manager.go` (new seed field, setter, use in `CARootPath`)
- Modify: `mobile/libbox.go` (new `SetFilterCASeed`)
- Test: `internal/filter/manager_mitm_test.go` (add one test)

**Interfaces:**
- Consumes: `EnsureRoot(filterDir, seed string)` from Task 1.
- Produces:
  - `func (m *Manager) SetCASeed(seed string)` — stores seed for later CA generation.
  - `func SetFilterCASeed(dataDir, seed string) error` (package `mobile`) — Kotlin-facing entry point.

- [ ] **Step 1: Write the failing test**

Add to `internal/filter/manager_mitm_test.go`:

```go
func TestManager_SetCASeed_ReproducesCA(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	m1 := NewManager(dir1)
	m1.SetCASeed("stable-seed-xyz")
	p1, err := m1.CARootPath()
	if err != nil {
		t.Fatalf("m1 CARootPath: %v", err)
	}
	cert1, _ := os.ReadFile(p1)

	m2 := NewManager(dir2)
	m2.SetCASeed("stable-seed-xyz")
	p2, err := m2.CARootPath()
	if err != nil {
		t.Fatalf("m2 CARootPath: %v", err)
	}
	cert2, _ := os.ReadFile(p2)

	if !bytes.Equal(cert1, cert2) {
		t.Fatal("same seed via manager produced different CA certs")
	}
}
```

Ensure `"bytes"` and `"os"` are imported in that test file (add if missing).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/filter/ -run TestManager_SetCASeed_ReproducesCA`
Expected: FAIL — `m1.SetCASeed undefined`.

- [ ] **Step 3: Add the seed field, setter, and thread it through**

In `internal/filter/manager.go`, add a field to the `Manager` struct (near the other atomic/state fields around line 78):

```go
	// caSeed, when non-empty, makes root CA generation deterministic so a
	// reinstalled app recreates the exact CA already trusted by the system.
	// Guarded by mu. Set once at startup via SetCASeed before any CA access.
	caSeed string
```

Add the setter (place it just above `CARootPath`, before line 436):

```go
// SetCASeed records the stable device seed used for deterministic CA
// generation. Call before the first CARootPath()/StartMITM() so the CA is
// created deterministically rather than randomly.
func (m *Manager) SetCASeed(seed string) {
	m.mu.Lock()
	m.caSeed = seed
	m.mu.Unlock()
}
```

Update `CARootPath` to read the seed under the lock and pass it:

```go
func (m *Manager) CARootPath() (string, error) {
	m.mu.RLock()
	seed := m.caSeed
	m.mu.RUnlock()
	root, err := ca.EnsureRoot(m.FilterDir(), seed)
	if err != nil {
		return "", err
	}
	return root.CertificatePath, nil
}
```

(Verify `m.mu` is a `sync.RWMutex`; the existing `IsMITMRunning` uses `RLock`, so it is.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/filter/ -run TestManager_SetCASeed_ReproducesCA`
Expected: PASS.

- [ ] **Step 5: Add the mobile entry point**

In `mobile/libbox.go`, add after `FilterCARootPath` (after line 1344):

```go
// SetFilterCASeed records a stable, device-scoped seed (the Android side
// passes Settings.Secure.ANDROID_ID) so the root CA is generated
// deterministically. Reinstalling the app then recreates the byte-identical
// CA already trusted by the system, avoiding a re-install prompt and
// duplicate trust-store entries. Must be called before the first
// FilterCARootPath/StartFilterProxy. A blank seed leaves generation random.
func SetFilterCASeed(dataDir, seed string) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("dataDir is required")
	}
	getFilterManager(dataDir).SetCASeed(seed)
	return nil
}
```

(`strings` and `fmt` are already imported in this file.)

- [ ] **Step 6: Verify the whole module builds and tests pass**

Run: `go build ./... && go test ./internal/filter/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/manager.go mobile/libbox.go internal/filter/manager_mitm_test.go
git commit -m "feat(ca): thread device seed through manager and mobile API"
```

---

### Task 3: Wire the seed and add stale-entry counting (Kotlin)

**Files:**
- Modify: `android/app/src/main/java/com/resultv/android/vpn/CertStore.kt` (new `staleEntryCount`, `isResultVCommonName` helper, `applySeed`)
- Modify: `android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt:105-111` (call `CertStore.applySeed` after capturing `hwidSource`)
- Test: `android/app/src/test/java/com/resultv/android/vpn/CertStoreTest.kt` (new file)

**Interfaces:**
- Consumes: `Mobile.setFilterCASeed(dataDir, seed)` from Task 2 (gomobile lowercases the exported Go name's first letter).
- Produces:
  - `CertStore.applySeed(dataDir: String, seed: String)` — forwards to Go; call once at startup.
  - `CertStore.staleEntryCount(dataDir: String): Int` — count of user CAs named like ours but not byte-equal to the current CA.
  - `CertStore.isResultVCommonName(subjectDn: String?): Boolean` — pure, testable predicate.

- [ ] **Step 1: Write the failing test**

Create `android/app/src/test/java/com/resultv/android/vpn/CertStoreTest.kt`:

```kotlin
package com.resultv.android.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CertStoreTest {

    @Test fun matchesOurCommonName() {
        assertTrue(CertStore.isResultVCommonName("CN=ResultV AdBlock Root CA,O=ResultV"))
        assertTrue(CertStore.isResultVCommonName("CN=ResultV AdBlock Root CA"))
    }

    @Test fun ignoresOtherAuthorities() {
        assertFalse(CertStore.isResultVCommonName("CN=DigiCert Global Root CA"))
        assertFalse(CertStore.isResultVCommonName("CN=Some ResultV Lookalike"))
        assertFalse(CertStore.isResultVCommonName(null))
        assertFalse(CertStore.isResultVCommonName(""))
    }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `.\gradlew.bat :app:testDebugUnitTest --tests "com.resultv.android.vpn.CertStoreTest"`
Expected: FAIL — unresolved reference `isResultVCommonName`.

- [ ] **Step 3: Add the helpers to CertStore**

In `android/app/src/main/java/com/resultv/android/vpn/CertStore.kt`, add these members to the `CertStore` object (after `isInstalled`, before `loadOurCa`). Match the file's existing constant for the CA name — it must equal the Go `caCN`:

```kotlin
    /** Subject CommonName of our root CA — must match the Go side's caCN. */
    private const val CA_COMMON_NAME = "ResultV AdBlock Root CA"

    /**
     * Records the stable device seed so the Go side regenerates the exact same
     * root CA after a reinstall. Safe to call repeatedly; forwards a blank seed
     * unchanged (Go then keeps random generation).
     */
    fun applySeed(dataDir: String, seed: String) {
        runCatching { Mobile.setFilterCASeed(dataDir, seed) }
            .onFailure { Log.w(TAG, "Couldn't set CA seed", it) }
    }

    /**
     * How many user-installed CAs carry our CommonName but are NOT byte-equal
     * to the CA we currently use. These are stale leftovers from earlier random
     * generations, piling up in the system trust store across reinstalls.
     *
     * Blocking KeyStore + filesystem I/O — call from a background thread.
     * Returns 0 on any error, matching [isInstalled]'s fail-safe stance.
     */
    fun staleEntryCount(dataDir: String): Int = try {
        val ours = loadOurCa(dataDir)
        val store = KeyStore.getInstance("AndroidCAStore").apply { load(null, null) }
        store.aliases().asSequence()
            .filter { it.startsWith("user:") }
            .count { alias ->
                val cert = store.getCertificate(alias) as? X509Certificate
                cert != null &&
                    isResultVCommonName(cert.subjectX500Principal.name) &&
                    !cert.encoded.contentEquals(ours)
            }
    } catch (t: Throwable) {
        Log.w(TAG, "Couldn't count stale CA entries", t)
        0
    }

    /**
     * True when an X.500 subject DN names our root CA. Pure and testable — the
     * trust-store iteration around it is Android-only and untested, like
     * [isInstalled].
     */
    fun isResultVCommonName(subjectDn: String?): Boolean =
        subjectDn?.split(",")?.any { it.trim() == "CN=$CA_COMMON_NAME" } ?: false
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `.\gradlew.bat :app:testDebugUnitTest --tests "com.resultv.android.vpn.CertStoreTest"`
Expected: PASS.

- [ ] **Step 5: Call applySeed at startup**

In `android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt`, inside `init` right after `hwidSource` is assigned (after line 110), add:

```kotlin
        CertStore.applySeed(app.filesDir.absolutePath, hwidSource)
```

This reuses the already-captured `ANDROID_ID` (`hwidSource`) as the CA seed, before any wizard/settings call reaches `CertStore.isInstalled` or the Go CA generator.

- [ ] **Step 6: Build the Android module**

Run: `.\gradlew.bat :app:assembleDebug`
Expected: BUILD SUCCESSFUL (confirms `Mobile.setFilterCASeed` exists in the bound `.aar` and the new code compiles).

Note: if the gomobile `.aar` is committed/prebuilt rather than rebuilt by Gradle, regenerate it first with the project's existing bind command (check `scripts/` or the Go/gomobile build step) so `setFilterCASeed` is present, then re-run this step.

- [ ] **Step 7: Commit**

```bash
git add android/app/src/main/java/com/resultv/android/vpn/CertStore.kt android/app/src/main/java/com/resultv/android/vpn/SettingsRepository.kt android/app/src/test/java/com/resultv/android/vpn/CertStoreTest.kt
git commit -m "feat(ca): seed CA generation and count stale trust-store entries"
```

---

### Task 4: Stale-entry banner in the cert UI (Kotlin/Compose)

**Files:**
- Modify: `android/app/src/main/res/values/strings.xml` (+ any sibling `values-*/strings.xml`)
- Modify: `android/app/src/main/java/com/resultv/android/ui/screens/CertWizardScreen.kt` (banner + count check)

**Interfaces:**
- Consumes: `CertStore.staleEntryCount(dataDir)` from Task 3.
- Produces: no new cross-task symbols (UI only).

- [ ] **Step 1: Add the strings**

In `android/app/src/main/res/values/strings.xml`, add (place near the other cert-wizard strings):

```xml
    <string name="cert_stale_banner_title">Old certificates found</string>
    <string name="cert_stale_banner_body">%1$d leftover \"ResultV AdBlock Root CA\" certificate(s) from a previous install are still in your system trust store. Remove the extras to keep things tidy.</string>
    <string name="cert_stale_banner_action">Open trusted credentials</string>
```

For each existing `android/app/src/main/res/values-*/strings.xml` (e.g. a Russian `values-ru/`), add the same three keys with translated values. Check which locale folders exist first:

Run: `ls android/app/src/main/res/ | grep values`

If only `values/` exists, this sub-step is done.

- [ ] **Step 2: Verify the strings compile**

Run: `.\gradlew.bat :app:processDebugResources`
Expected: BUILD SUCCESSFUL (no missing-translation or malformed-resource errors).

- [ ] **Step 3: Add the banner composable**

In `android/app/src/main/java/com/resultv/android/ui/screens/CertWizardScreen.kt`, add a composable that checks the count off the main thread and renders a warning card with a deep link to the system trusted-credentials screen. Place it near `InstallWatcher` (around line 248):

```kotlin
/**
 * Warns when the system trust store holds leftover "ResultV" CAs from earlier
 * installs. Android does not let an app delete a user CA programmatically, so
 * this points the user at Settings and lets them clean up.
 */
@Composable
private fun StaleCertBanner(dataDir: String) {
    val context = LocalContext.current
    var count by remember { mutableStateOf(0) }

    LaunchedEffect(dataDir) {
        count = withContext(Dispatchers.IO) { CertStore.staleEntryCount(dataDir) }
    }

    if (count <= 0) return

    Card(
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.errorContainer,
        ),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp)) {
            Text(
                text = stringResource(R.string.cert_stale_banner_title),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onErrorContainer,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                text = stringResource(R.string.cert_stale_banner_body, count),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onErrorContainer,
            )
            Spacer(Modifier.height(8.dp))
            TextButton(onClick = {
                runCatching {
                    context.startActivity(Intent(Settings.ACTION_SECURITY_SETTINGS))
                }
            }) {
                Text(stringResource(R.string.cert_stale_banner_action))
            }
        }
    }
}
```

- [ ] **Step 4: Render the banner in the wizard**

In `CertWizardScreen.kt`, call `StaleCertBanner(dataDir)` inside the wizard's top-level column where `dataDir` is in scope (the same `dataDir` passed to `CertStore.isInstalled` at line 259). Add it above the current step content so the warning is visible on the install screens.

Ensure imports exist (add any missing): `androidx.compose.runtime.getValue`, `setValue`, `mutableStateOf`, `remember`, `LaunchedEffect`, `androidx.compose.material3.Card`, `CardDefaults`, `TextButton`, `android.content.Intent`, `android.provider.Settings`, `androidx.compose.ui.res.stringResource`, `com.resultv.android.R`. Reuse whatever `Dispatchers`/`withContext`/`LocalContext` imports the file already has (they are present for `InstallWatcher`).

- [ ] **Step 5: Build the Android module**

Run: `.\gradlew.bat :app:assembleDebug`
Expected: BUILD SUCCESSFUL.

- [ ] **Step 6: Commit**

```bash
git add android/app/src/main/res/values/strings.xml android/app/src/main/java/com/resultv/android/ui/screens/CertWizardScreen.kt
git commit -m "feat(ca): warn about leftover ResultV CA entries in the wizard"
```

If sibling locale `strings.xml` files were edited in Step 1, include them in the `git add`.

---

## Final Verification

- [ ] Run `go test ./internal/filter/... ./mobile/...` — all pass.
- [ ] Run `.\gradlew.bat :app:testDebugUnitTest` — all unit tests pass.
- [ ] Manual device check (if hardware available): install, add CA, uninstall, reinstall → wizard reports CA already trusted with no re-install step; deliberately install a second (older) CA → banner reports the leftover count.

## Notes for the Implementer

> **IMPLEMENTATION UPDATE (as built):** The `rsa.GenerateKey(drbg, …)` calls shown in Task 1's code blocks were superseded during implementation. On Go 1.26, `crypto/rsa` ignores a custom `io.Reader` (`crypto/internal/rand.CustomReader` + `MaybeReadByte`), so it cannot be made deterministic. The shipped code (`internal/filter/ca/ca.go`) instead generates the RSA key with a `math/big` prime search (`generateDeterministicKey`/`randPrime`) mirroring the classic `crypto/rand.Prime`, fed by the same ChaCha20 DRBG. This deviation was reviewed and approved. The corrected design of record is in `docs/superpowers/specs/2026-07-23-deterministic-ca-design.md`.

- **Why ChaCha20 and not HKDF directly:** `hkdf.New` returns a reader capped at 255×hash-length (8160 bytes for SHA-256), which RSA-2048 prime rejection sampling can exceed, causing an EOF mid-keygen. ChaCha20 over a zero plaintext is an unbounded, deterministic stream.
- **Accepted fragility:** deterministic reproduction depends on the stability of `math/big.ProbablyPrime` (its Miller-Rabin base selection is deterministic and consumes no external randomness) plus the ChaCha20 stream — more robust than relying on `rsa.GenerateKey` internals. Because this only runs when `ca.key` is missing (a reinstall), a future change in `ProbablyPrime` would at worst degrade to today's behavior (a fresh CA needing install) — no regression. Do not add complexity to defend against this.
- **Existing random-CA users:** their first reinstall after shipping this yields one deterministic duplicate; the Task 4 banner surfaces it for manual cleanup. This is expected, not a bug.
