package com.aitrack.server.domain.service;

import java.util.List;

/**
 * Pure domain value object carrying the tunable thresholds of the validation chain.
 *
 * <p>The infrastructure config layer ({@code infrastructure.app}) maps the
 * {@code @ConfigurationProperties} settings onto this record so the domain
 * stays free of config-file coupling. Mirrors the Go server's
 * {@code service.ValidationPolicy} struct.
 *
 * @param rateLimitPerHour   max accepted edits per token+file per rolling hour
 * @param maxAddedLines      added_lines threshold above which an edit is flagged oversized
 * @param repoWhitelistUrls  allowed repo URLs; empty/null means no whitelist enforcement
 * @param enforceWhitelist   when true, a repo outside the whitelist is hard-rejected;
 *                           when false, it is only soft-flagged
 */
public record ValidationPolicy(
    long rateLimitPerHour,
    long maxAddedLines,
    List<String> repoWhitelistUrls,
    boolean enforceWhitelist
) {}
