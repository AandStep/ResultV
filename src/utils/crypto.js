// === AES-256-GCM шифрование через Web Crypto API ===

const ITERATIONS = 100000;
const SALT_LENGTH = 16;
const IV_LENGTH = 12;

/**
 * Derive an AES-256-GCM key from a password using PBKDF2
 */
async function deriveKey(password, salt) {
  const enc = new TextEncoder();
  const keyMaterial = await crypto.subtle.importKey(
    "raw",
    enc.encode(password),
    "PBKDF2",
    false,
    ["deriveKey"],
  );

  return crypto.subtle.deriveKey(
    { name: "PBKDF2", salt, iterations: ITERATIONS, hash: "SHA-256" },
    keyMaterial,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
}

/**
 * Encrypt data with a user-provided password.
 * Format: base64( salt[16] + iv[12] + ciphertext + authTag )
 */
export const encryptWithPassword = async (data, pwd) => {
  try {
    const enc = new TextEncoder();
    const plaintext = enc.encode(JSON.stringify(data));
    const salt = crypto.getRandomValues(new Uint8Array(SALT_LENGTH));
    const iv = crypto.getRandomValues(new Uint8Array(IV_LENGTH));
    const key = await deriveKey(pwd, salt);

    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv },
      key,
      plaintext,
    );

    // Concatenate: salt + iv + encrypted data (includes authTag)
    const result = new Uint8Array(
      salt.length + iv.length + ciphertext.byteLength,
    );
    result.set(salt, 0);
    result.set(iv, salt.length);
    result.set(new Uint8Array(ciphertext), salt.length + iv.length);

    // Convert to base64
    let binary = "";
    for (let i = 0; i < result.length; i++) {
      binary += String.fromCharCode(result[i]);
    }
    return btoa(binary);
  } catch (e) {
    console.error("Encryption error:", e);
    return null;
  }
};

/**
 * Decrypt data with a user-provided password.
 */
export const decryptWithPassword = async (encodedStr, pwd) => {
  try {
    const binary = atob(encodedStr);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }

    const salt = bytes.slice(0, SALT_LENGTH);
    const iv = bytes.slice(SALT_LENGTH, SALT_LENGTH + IV_LENGTH);
    const ciphertext = bytes.slice(SALT_LENGTH + IV_LENGTH);

    const key = await deriveKey(pwd, salt);

    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv },
      key,
      ciphertext,
    );

    const text = new TextDecoder().decode(decrypted);
    return JSON.parse(text);
  } catch (e) {
    return null;
  }
};
