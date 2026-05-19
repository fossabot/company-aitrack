package com.aitrack.server.adapter.handler;

import com.aitrack.server.application.TokenService;
import com.aitrack.server.domain.model.TokenEntity;
import com.aitrack.server.domain.service.SignatureService;
import com.aitrack.server.infrastructure.config.AiTrackProperties;
import jakarta.servlet.http.HttpServletRequest;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.web.server.ResponseStatusException;

import java.time.Instant;

/**
 * Steps 1-3 of the hardening chain, applied to every signed request.
 */
@Component
@RequiredArgsConstructor
public class RequestAuthHelper {

    private final TokenService tokenService;
    private final SignatureService signatureService;
    private final AiTrackProperties props;

    /**
     * Resolves the active token from the Authorization header.
     * Throws 401 if the token is missing or inactive.
     */
    public TokenEntity resolveToken(HttpServletRequest request) {
        String authHeader = request.getHeader("Authorization");
        if (authHeader == null || !authHeader.startsWith("Bearer ")) {
            throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, "missing bearer token");
        }
        String rawToken = authHeader.substring("Bearer ".length()).trim();
        return tokenService.findActiveToken(rawToken)
            .orElseThrow(() -> new ResponseStatusException(HttpStatus.UNAUTHORIZED, "invalid or inactive token"));
    }

    /**
     * Validates X-AiTrack-Timestamp (step 2) and X-AiTrack-Signature (step 3).
     * Must be called after resolveToken().
     */
    public void validateRequestSignature(HttpServletRequest request, TokenEntity token, byte[] rawBodyBytes) {
        String tsHeader = request.getHeader("X-AiTrack-Timestamp");
        if (tsHeader == null || tsHeader.isBlank()) {
            throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, "missing X-AiTrack-Timestamp");
        }

        long ts;
        try {
            ts = Long.parseLong(tsHeader.trim());
        } catch (NumberFormatException e) {
            throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, "invalid X-AiTrack-Timestamp");
        }

        long nowSec = Instant.now().getEpochSecond();
        if (Math.abs(nowSec - ts) > props.getTimestampWindowSeconds()) {
            throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, "timestamp out of window");
        }

        String sigHeader = request.getHeader("X-AiTrack-Signature");
        if (sigHeader == null || sigHeader.isBlank()) {
            throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, "missing X-AiTrack-Signature");
        }

        String expected = signatureService.computeRequestSignature(token.getHmacSecret(), tsHeader.trim(), rawBodyBytes);
        if (!constantTimeEquals(expected, sigHeader.trim())) {
            throw new ResponseStatusException(HttpStatus.UNAUTHORIZED, "invalid X-AiTrack-Signature");
        }
    }

    private static boolean constantTimeEquals(String a, String b) {
        if (a == null || b == null) return false;
        byte[] aBytes = a.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        byte[] bBytes = b.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        return java.security.MessageDigest.isEqual(aBytes, bBytes);
    }
}
