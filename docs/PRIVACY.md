# aitrack Privacy Notice

Version: v1.1 · Last updated: 2026-05-19

---

## 1. Overview

This document explains what data aitrack collects, why, where it is stored, who can see it, and what control you have over it.

**What aitrack is:** A self-hosted AI coding governance platform. It hooks into file-edit events from AI coding tools (Claude Code, Codex CLI, Cursor) and records what changes those tools make to your codebase. The goal is to give teams a factual picture of how AI tools are being used in development.

**Why this document exists:** Any tool that records developer activity should be transparent about what it does. This is not a legal disclaimer — it is a direct answer to the question "where does my data go?"

**Self-hosted by design:** aitrack has no cloud component. When you deploy aitrack, you are the operator. All data stays inside your own infrastructure. No data is ever sent to the aitrack project maintainers or any third-party service.

---

## 2. What We Collect

Each record corresponds to one file-edit event triggered by an AI tool. These are all the fields collected:

| Data item | What is collected | Why it is needed | Where it is stored |
|-----------|-------------------|------------------|--------------------|
| **Change diff (diff_hunk)** | The actual added/removed code lines, in standard unified diff format — only the changed section, not the full file | Analyze what code the AI actually produced | Local SQLite + server database |
| **File path (file_path)** | Relative path, e.g. `src/main/java/com/example/Service.java` | Understand which modules and layers are most affected | Local SQLite + server database |
| **Lines added (added_lines)** | Actual new lines introduced by this edit | Quantify AI code contribution | Local SQLite + server database |
| **Lines removed (removed_lines)** | Actual lines deleted by this edit | Quantify AI-driven refactoring | Local SQLite + server database |
| **Timestamp** | Unix seconds, when the event occurred | Time-based usage analysis | Local SQLite + server database |
| **Repository URL (repo_url)** | Git remote origin URL, e.g. `git@github.com:org/repo.git` | Group records by project | Local SQLite + server database |
| **Branch (branch)** | Current Git branch name | Distinguish trunk from feature work | Local SQLite + server database |
| **Commit hash (current_sha)** | HEAD commit SHA at the time of the edit | Associate edits with a specific code snapshot | Local SQLite + server database |
| **Hostname** | OS hostname of the machine that made the edit, e.g. `MacBook-Pro.local` | Identify which machine produced a record when one credential is used across multiple machines; not used for access control | Local SQLite + server database |
| **AI tool type (tool)** | One of: `claude`, `codex`, `cursor` | Distinguish usage patterns by tool | Local SQLite + server database |
| **Token identifier (token_key)** | The token portion of the credential assigned by an admin, format `aitrack_<hex>` | Attribute records to a specific developer seat | Local SQLite only (used for local filtering, not sent with upload payload) |
| **Device ID (device_id)** | UUIDv4 generated on first run, persisted to local config | Distinguish multiple devices using the same credential | Local SQLite + server database |
| **Record signature (record_sig)** | HMAC-SHA256 signature binding all the above fields | Detect if a local record has been tampered with or forged | Local SQLite + server database |

**A note on diff_hunk:** This is the diff of the changed section, not the full file. If the AI modified a function, you get the before/after for that function — nothing else from the file is included. The diff algorithm uses Myers/LCS minimum edit distance, so diffs are as small as possible.

---

## 3. What We Do Not Collect

The following data is **outside the collection scope**, and here is how that is enforced technically:

| Not collected | How it is enforced |
|---------------|--------------------|
| **Full file contents** | Only `diff_hunk` (the changed section) is stored. The capture process does not read or store anything beyond the diff payload provided by the tool hook. |
| **Prompt text** | v1.1 through Phase 3: not collected at all. The capture entry point only processes the file-edit event JSON; the prompt never appears in this payload. |
| **Passwords, private keys, certificates** | The capture flow includes a file path plausibility check that automatically skips files matching patterns such as: `*.key`, `*.pem`, `*.pfx`, `*.p12`, `*.env`, `*secret*`, `*password*`, and similar sensitive file names. No record is created for these paths. |
| **AI conversation history** | aitrack hooks file-edit events only. Conversation history from Claude Code, Codex, or Cursor does not pass through aitrack at any point. |
| **The HMAC secret portion of credentials** | A credential combines `<token>-<hmac_secret>`. The `hmac_secret` is only used locally to compute signatures and is never sent over the network. |
| **Personal identity information unrelated to coding** | aitrack does not access any system outside the development environment. |

---

## 4. How Data Is Stored

### Local storage (client side)

- Directory: `~/.aitrack/`
- Database: `~/.aitrack/records.db` (SQLite)
  - File permissions: 0600 — readable only by the owning user
  - All records are written here before upload; you can inspect them with `aitrack inspect`
- Config file: `~/.aitrack/config.toml`
  - File permissions: 0600
  - Contains: API URL, credential (token + hmac_secret combined), device ID

### Server-side storage

- Database: PostgreSQL (ParadeDB) or SQLite, depending on your deployment mode
- Only administrators with direct database access can query raw records
- All data remains within your own infrastructure — no external service involved

### Encryption

- The `hmac_secret` portion of each credential is stored on the server encrypted with AES-256-GCM. Even an admin with database access cannot read it in plaintext.
- Tokens are stored server-side as SHA-256 hashes. The original credential is returned only once, at issuance.
- Records in transit are protected by dual HMAC-SHA256 signatures (per-record `record_sig` + per-request `X-AiTrack-Signature`), so any tampering in transit can be detected.

---

## 5. Who Can Access the Data

| Role | What they can see | How |
|------|-------------------|-----|
| **You (the developer)** | All records generated on your machine, including diff content | `aitrack inspect --limit 100` — reads directly from local SQLite, no network needed |
| **Administrator** | All records across all developers (attributed by token) | Server database or admin API (requires `X-Admin-Key`) |
| **Other team members (non-admin)** | No direct access to server-side records | — |
| **Third parties** | Nothing | — |

**On third-party access:** Because aitrack is self-hosted, all data transmission happens within your own network. aitrack calls no external APIs and sends no data to the aitrack project maintainers or any vendor. If you deploy on a cloud VM, your cloud provider's standard policies apply to the infrastructure, but aitrack itself does not route data externally.

---

## 6. Data Retention

**Current version (v1.1) has no automatic expiry.** Records uploaded to the server persist until explicitly deleted.

How to clean up:

- **Local records:** `aitrack clean --all` removes already-synced records from the local SQLite database.
- **Server records:** The administrator can delete records by token key, time range, or repository using direct database operations or the admin API.

A configurable server-side TTL is planned for a future version. This document will be updated when that is available.

---

## 7. Your Rights as a Self-Hosted User

Because you self-host aitrack, you control everything:

**Inspect your local data:**
```bash
aitrack inspect --limit 100      # View the most recent 100 records (includes diff content)
aitrack inspect --pending        # View records not yet uploaded
aitrack stats                    # Aggregated counts by tool and repository
aitrack status                   # Check which tool hooks are installed
```

**Remove hooks at any time:**
```bash
aitrack remove --claude          # Remove the Claude Code hook
aitrack remove --codex           # Remove the Codex CLI hook
aitrack remove --cursor          # Remove the Cursor hook
```

Once a hook is removed, no new records are created by that tool.

**Delete your data:** As a self-hosted operator, you have full access to both the local SQLite file (`~/.aitrack/records.db`) and the server database. You can delete records directly or ask your admin to do so. There is no lock-in or data held outside your infrastructure.

**Stop collection entirely:** Uninstall all hooks (`aitrack remove --claude --codex --cursor`) or remove the aitrack binary. Existing records in the database are not affected until you delete them manually.

---

## 8. A Note on Prompt Data

**Current version (v1.1, Phase 1–3): prompts are not collected, at all.**

The aitrack capture hook fires after a file-edit event completes. At that point, the prompt is no longer in the processing flow. The client code does not read or store any prompt text.

**Phase 4 (not yet implemented):** There is a plan to collect a prompt summary hash — a one-way fingerprint used for semantic deduplication analysis — without collecting the prompt text itself. This is not yet part of any released version. Before implementing it, this document will be updated and users will be notified in advance.

---

## 9. Security Mechanisms

**Dual HMAC-SHA256 signatures:**

- Per-record signature (`record_sig`): Computed when a record is written to the local database. It binds the device ID, hostname, timestamp, tool, file path, repository URL, commit SHA, line counts, and a hash of the diff. The server rejects records with invalid signatures.
- Per-request signature (`X-AiTrack-Signature`): Covers the entire upload request body and a timestamp. Prevents replay attacks and in-transit modification.

**Credential storage:**

- The `hmac_secret` is stored server-side with AES-256-GCM encryption.
- Tokens are stored as SHA-256 hashes. The plaintext credential is returned only once at issuance and cannot be recovered from the server afterward.

**Path filtering:** The capture flow (step 8 of 10) checks each file path for plausibility. Files matching sensitive name patterns are automatically skipped — no record is written, no diff is computed.

**Rate limiting:** The server enforces a limit of 30 records per (token, file_path) pair per hour to prevent gaming via edit count inflation.

**Heartbeat:** The client periodically reports hook installation status to the server, so administrators can detect if hooks have been silently removed (hardening point H3).

**Local database permissions:** Both `~/.aitrack/records.db` and `~/.aitrack/config.toml` are created with 0600 permissions. On multi-user machines, other OS users cannot read these files.

---

## 10. Contact and Feedback

If you have questions about how data is handled, found a security issue, or want to suggest changes to this document:

- Open an issue in the aitrack GitHub repository
- Or contact the repository maintainers directly

For security vulnerabilities, please follow the responsible disclosure process described in `SECURITY.md` at the repository root.

---

*This document is versioned alongside the software. If the collection scope changes — particularly for the planned Phase 4 prompt summary hash — this document will be updated before the release ships.*
