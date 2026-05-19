//! Self-update logic for the aitrack binary.
//!
//! Flow:
//!   1. Detect current platform to build the expected asset filename.
//!   2. Query the GitHub Releases API for the latest release.
//!   3. Find the matching binary asset and its companion `.sig` file.
//!   4. Download both files.
//!   5. Verify the ed25519 signature over the binary bytes.
//!   6. On success, atomically replace the current executable.

use anyhow::{bail, Context, Result};
use base64::{engine::general_purpose::STANDARD as B64, Engine as _};
use ed25519_dalek::{Signature, VerifyingKey};
use serde::Deserialize;
use std::fs;

pub const GITHUB_RELEASES_API: &str =
    "https://api.github.com/repos/MapleEve/company-aitrack/releases/latest";

/// ed25519 public key — placeholder until release signing is configured.
/// Replace with the actual base64-encoded 32-byte verifying key before release.
/// Generate with: `ed25519-dalek::SigningKey::generate(&mut OsRng).verifying_key()`
/// then base64-encode the raw 32-byte representation.
pub const ED25519_PUBKEY_BASE64: &str =
    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="; // 32-byte placeholder — replace before release

// ---------------------------------------------------------------------------
// GitHub Releases API response types
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
struct Release {
    tag_name: String,
    assets: Vec<Asset>,
}

#[derive(Deserialize)]
struct Asset {
    name: String,
    browser_download_url: String,
}

// ---------------------------------------------------------------------------
// Platform detection
// ---------------------------------------------------------------------------

/// Build the asset filename stem for the current platform.
/// Format: `aitrack-{arch}-{os}` on Unix, `aitrack-{arch}-{os}.exe` on Windows.
pub fn platform_target_string() -> String {
    let arch = match std::env::consts::ARCH {
        "aarch64" => "aarch64",
        "x86_64" => "x86_64",
        other => other,
    };
    let os = match std::env::consts::OS {
        "macos" => "apple-darwin",
        "linux" => "unknown-linux-musl",
        "windows" => "pc-windows-msvc",
        other => other,
    };
    if std::env::consts::OS == "windows" {
        format!("aitrack-{arch}-{os}.exe")
    } else {
        format!("aitrack-{arch}-{os}")
    }
}

// ---------------------------------------------------------------------------
// Signature verification
// ---------------------------------------------------------------------------

/// Verify a raw 64-byte ed25519 `signature` over `message` using `pubkey_b64`.
fn verify_signature(pubkey_b64: &str, message: &[u8], signature_b64: &str) -> Result<()> {
    let key_bytes = B64
        .decode(pubkey_b64)
        .context("failed to base64-decode public key")?;
    if key_bytes.len() != 32 {
        bail!(
            "public key must be 32 bytes, got {}",
            key_bytes.len()
        );
    }
    let key_arr: [u8; 32] = key_bytes.try_into().unwrap();
    let verifying_key =
        VerifyingKey::from_bytes(&key_arr).context("invalid ed25519 public key bytes")?;

    let sig_bytes = B64
        .decode(signature_b64.trim())
        .context("failed to base64-decode signature")?;
    if sig_bytes.len() != 64 {
        bail!("signature must be 64 bytes, got {}", sig_bytes.len());
    }
    let sig_arr: [u8; 64] = sig_bytes.try_into().unwrap();
    let signature = Signature::from_bytes(&sig_arr);

    use ed25519_dalek::Verifier as _;
    verifying_key
        .verify(message, &signature)
        .context("ed25519 signature verification failed")?;
    Ok(())
}

// ---------------------------------------------------------------------------
// HTTP helpers (blocking)
// ---------------------------------------------------------------------------

fn http_get_bytes(url: &str) -> Result<Vec<u8>> {
    let client = reqwest::blocking::Client::builder()
        .user_agent("aitrack-updater/1")
        .build()
        .context("failed to build HTTP client")?;
    let resp = client
        .get(url)
        .send()
        .with_context(|| format!("GET {url} failed"))?;
    if !resp.status().is_success() {
        bail!("GET {url} returned HTTP {}", resp.status());
    }
    resp.bytes()
        .map(|b| b.to_vec())
        .context("failed to read response body")
}

fn http_get_text(url: &str) -> Result<String> {
    String::from_utf8(http_get_bytes(url)?).context("response body is not valid UTF-8")
}

// ---------------------------------------------------------------------------
// Atomic binary replacement
// ---------------------------------------------------------------------------

fn replace_current_exe(new_bytes: &[u8]) -> Result<()> {
    let current = std::env::current_exe().context("cannot determine current executable path")?;
    let tmp_path = current.with_extension("tmp");
    fs::write(&tmp_path, new_bytes)
        .with_context(|| format!("failed to write {}", tmp_path.display()))?;

    // Make the new binary executable on Unix.
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let perms = fs::Permissions::from_mode(0o755);
        fs::set_permissions(&tmp_path, perms)
            .context("failed to set executable permission on new binary")?;
    }

    fs::rename(&tmp_path, &current).with_context(|| {
        format!(
            "failed to replace {} with {}",
            current.display(),
            tmp_path.display()
        )
    })?;
    Ok(())
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

pub fn run_update() -> Result<()> {
    let target = platform_target_string();
    println!("Checking for updates (platform: {target})...");

    // Fetch latest release metadata.
    let release_json = http_get_text(GITHUB_RELEASES_API)
        .context("failed to reach GitHub Releases API")?;
    let release: Release =
        serde_json::from_str(&release_json).context("failed to parse GitHub release JSON")?;

    println!("Latest release: {}", release.tag_name);

    // Find binary asset.
    let bin_asset = release
        .assets
        .iter()
        .find(|a| a.name == target)
        .with_context(|| format!("no asset named '{target}' in latest release"))?;

    // Find companion .sig asset.
    let sig_name = format!("{target}.sig");
    let sig_asset = release
        .assets
        .iter()
        .find(|a| a.name == sig_name)
        .with_context(|| format!("no signature asset named '{sig_name}' in latest release"))?;

    println!("Downloading {}...", bin_asset.name);
    let bin_bytes =
        http_get_bytes(&bin_asset.browser_download_url).context("failed to download binary")?;

    println!("Downloading {}...", sig_asset.name);
    let sig_text =
        http_get_text(&sig_asset.browser_download_url).context("failed to download signature")?;

    // Verify before touching the filesystem.
    println!("Verifying ed25519 signature...");
    verify_signature(ED25519_PUBKEY_BASE64, &bin_bytes, sig_text.trim())
        .context("signature verification failed — update aborted")?;

    println!("Signature OK. Replacing binary...");
    replace_current_exe(&bin_bytes)?;

    println!("Update complete. You are now on {}.", release.tag_name);
    Ok(())
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    /// Smoke-test: platform detection must not panic and must return a non-empty string.
    #[test]
    fn test_platform_target_string() {
        let s = platform_target_string();
        assert!(!s.is_empty(), "platform_target_string() returned empty string");
        assert!(s.starts_with("aitrack-"), "expected 'aitrack-' prefix, got: {s}");
    }

    /// A tampered (all-zeros) signature must be rejected even against a valid keypair.
    #[test]
    fn test_sig_verification_rejects_bad_sig() {
        use ed25519_dalek::SigningKey;
        use rand::rngs::OsRng;

        // Generate a fresh real keypair for this test — do NOT use the placeholder key.
        let signing_key = SigningKey::generate(&mut OsRng);
        let verifying_key = signing_key.verifying_key();
        let pubkey_b64 = B64.encode(verifying_key.as_bytes());

        let message = b"test payload";

        // A tampered signature: 64 zero bytes — definitely not valid.
        let bad_sig = B64.encode([0u8; 64]);

        let result = verify_signature(&pubkey_b64, message, &bad_sig);
        assert!(
            result.is_err(),
            "expected tampered signature to be rejected, but it was accepted"
        );
    }

    /// A correct signature produced by a matching key must be accepted.
    #[test]
    fn test_sig_verification_accepts_valid_sig() {
        use ed25519_dalek::{Signer, SigningKey};
        use rand::rngs::OsRng;

        let signing_key = SigningKey::generate(&mut OsRng);
        let verifying_key = signing_key.verifying_key();
        let pubkey_b64 = B64.encode(verifying_key.as_bytes());

        let message = b"authentic payload";
        let signature: ed25519_dalek::Signature = signing_key.sign(message);
        let sig_b64 = B64.encode(signature.to_bytes());

        let result = verify_signature(&pubkey_b64, message, &sig_b64);
        assert!(result.is_ok(), "expected valid signature to be accepted: {result:?}");
    }

    /// A signature valid for one message must be rejected for a different message.
    #[test]
    fn test_sig_verification_rejects_wrong_message() {
        use ed25519_dalek::{Signer, SigningKey};
        use rand::rngs::OsRng;

        let signing_key = SigningKey::generate(&mut OsRng);
        let verifying_key = signing_key.verifying_key();
        let pubkey_b64 = B64.encode(verifying_key.as_bytes());

        let original = b"original message";
        let signature: ed25519_dalek::Signature = signing_key.sign(original);
        let sig_b64 = B64.encode(signature.to_bytes());

        let tampered = b"tampered message";
        let result = verify_signature(&pubkey_b64, tampered, &sig_b64);
        assert!(
            result.is_err(),
            "expected signature to be rejected for wrong message"
        );
    }
}
