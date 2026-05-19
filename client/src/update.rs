// client/src/update.rs — self-update with ed25519 signature verification
//
// Open-source version: fetches from GitHub Releases API.
// Company version: fetches from AITRACK_UPDATE_URL env var (or config).
//
// Public key is hardcoded — cannot be replaced without recompiling.

use std::env;

/// Hardcoded ed25519 public key (base64-encoded).
/// Generated with: `minisign -G -p aitrack.pub -s aitrack.sec`
/// This key is embedded at compile time and cannot be changed by users.
/// Replace with real key before first release.
const UPDATE_PUBLIC_KEY: &str = "PLACEHOLDER_ED25519_PUBKEY_BASE64";

const GITHUB_RELEASES_URL: &str =
    "https://api.github.com/repos/MapleEve/company-aitrack/releases/latest";

/// Self-update arguments.
#[derive(Debug)]
pub struct UpdateArgs {
    /// Check for updates without installing.
    pub check_only: bool,
    /// Force update even if already on latest version.
    pub force: bool,
}

/// Run the update command.
///
/// Flow:
/// 1. Determine update source (GitHub or AITRACK_UPDATE_URL).
/// 2. Fetch latest release metadata.
/// 3. Compare version with current (env!("CARGO_PKG_VERSION")).
/// 4. If newer (or --force): download binary + .sig file.
/// 5. Verify ed25519 signature with embedded public key.
/// 6. Atomically replace current binary.
pub fn run_update(args: &UpdateArgs) -> anyhow::Result<()> {
    let current_version = env!("CARGO_PKG_VERSION");
    let update_url = env::var("AITRACK_UPDATE_URL")
        .unwrap_or_else(|_| GITHUB_RELEASES_URL.to_string());

    // Suppress unused-constant warning until real implementation lands.
    let _ = UPDATE_PUBLIC_KEY;

    println!("Current version: v{}", current_version);
    println!("Update source: {}", update_url);

    if args.check_only {
        println!("[check-only] Would fetch latest version from {}", update_url);
        println!("Note: real update requires network access and write permission to current binary.");
        return Ok(());
    }

    // TODO: Implement full update flow after ed25519 key is generated.
    // Steps when implemented:
    // 1. GET {update_url} → parse "tag_name" and asset URLs
    // 2. Compare tag_name vs current_version
    // 3. Download binary asset + binary.sig asset
    // 4. Verify: ed25519::verify(public_key, binary_bytes, signature)
    // 5. Write to temp file, set +x, rename to replace current executable
    println!("Update functionality will be available in the next release.");
    println!("For now, please download manually from:");
    println!("  https://github.com/MapleEve/company-aitrack/releases");
    Ok(())
}
