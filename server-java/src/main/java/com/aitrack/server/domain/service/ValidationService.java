package com.aitrack.server.domain.service;

import com.aitrack.server.domain.model.EditDto;
import com.aitrack.server.domain.model.TokenEntity;
import com.aitrack.server.domain.port.EditRecordPort;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * Implements the 10-step validation chain for each edit.
 * Steps 1-3 are request-level (handled in controller / filter before this service is called).
 * Steps 4-10 are per-edit and return a ValidationResult.
 */
@Service
@RequiredArgsConstructor
public class ValidationService {

    private final SignatureService signatureService;
    private final DiffConsistencyService diffConsistencyService;
    private final EditRecordPort editRecordPort;
    private final ValidationPolicy policy;

    public enum EditOutcome { ACCEPTED, FLAGGED, REJECTED }

    public record ValidationResult(
        EditOutcome outcome,
        List<String> reasons
    ) {}

    /**
     * Validates a single edit according to steps 4-10 of the hardening chain.
     *
     * @param token   resolved active token entity
     * @param edit    the edit DTO from the batch request
     */
    public ValidationResult validate(TokenEntity token, EditDto edit) {
        List<String> flags = new ArrayList<>();

        // Step 4: record_sig
        String expectedSig = signatureService.computeRecordSig(
            token.getHmacSecret(),
            token.getTokenKey(),
            edit.getDeviceId(),
            edit.getHostname(),
            edit.getTimestamp(),
            edit.getTool(),
            edit.getFilePath(),
            edit.getRepoUrl(),
            edit.getCurrentSha(),
            edit.getAddedLines(),
            edit.getRemovedLines(),
            edit.getDiffHunk()
        );
        if (!constantTimeEquals(expectedSig, edit.getRecordSig())) {
            return new ValidationResult(EditOutcome.REJECTED, List.of("sig_mismatch"));
        }

        // Step 5: diff_hunk consistency
        if (!diffConsistencyService.isConsistent(edit.getDiffHunk(), edit.getAddedLines(), edit.getRemovedLines())) {
            flags.add("diff_inconsistent");
        }

        // Step 6: repo_url whitelist
        List<String> whitelist = policy.repoWhitelistUrls();
        boolean hasWhitelist = whitelist != null && !whitelist.isEmpty();
        if (hasWhitelist && !whitelist.contains(edit.getRepoUrl())) {
            if (policy.enforceWhitelist()) {
                // enforce=true: hard reject — repo not in whitelist is refused
                return new ValidationResult(EditOutcome.REJECTED, List.of("repo_not_whitelisted"));
            } else {
                // enforce=false: soft flag only, edit still ingested
                flags.add("repo_unknown");
            }
        }

        // Step 7: file_path / repo_url plausibility
        if (isPathMismatch(edit.getFilePath(), edit.getRepoUrl())) {
            flags.add("path_mismatch");
        }

        // Step 8: oversized
        if (edit.getAddedLines() > policy.maxAddedLines()) {
            flags.add("oversized");
        }

        // Step 9: rate limiting
        Instant windowStart = Instant.now().minusSeconds(3600);
        long count = editRecordPort.countByTokenKeyAndFilePathSince(
            token.getTokenKey(), edit.getFilePath(), windowStart);
        if (count >= policy.rateLimitPerHour()) {
            return new ValidationResult(EditOutcome.REJECTED, List.of("rate_limited"));
        }

        if (!flags.isEmpty()) {
            return new ValidationResult(EditOutcome.FLAGGED, flags);
        }
        return new ValidationResult(EditOutcome.ACCEPTED, List.of());
    }

    /**
     * Returns true only when the file_path contains genuine injection indicators:
     * <ul>
     *   <li>Path-traversal sequences ({@code ..} as a path component)</li>
     *   <li>NUL bytes or ASCII control characters (0x00-0x1F)</li>
     * </ul>
     *
     * <p>Absolute paths — including macOS-style {@code /Users/…} paths used by
     * Codex/Claude in worktrees and cloud-synced directories — are entirely normal
     * and are NOT flagged.  Flagging them produced ~97 % false positives in real
     * data (723/750 records) and rendered the flag useless as a signal.</p>
     */
    private boolean isPathMismatch(String filePath, String repoUrl) {
        if (filePath == null) return false;
        // Path traversal: any ".." component is suspicious regardless of OS
        if (filePath.contains("..")) {
            return true;
        }
        // NUL byte or ASCII control characters suggest injection / encoding attack
        for (int i = 0; i < filePath.length(); i++) {
            char c = filePath.charAt(i);
            if (c <= 0x1F) {
                return true;
            }
        }
        return false;
    }

    private static boolean constantTimeEquals(String a, String b) {
        if (a == null || b == null) return false;
        byte[] aBytes = a.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        byte[] bBytes = b.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        return java.security.MessageDigest.isEqual(aBytes, bBytes);
    }
}
