package com.aitrack.server.adapter.handler;

import com.aitrack.server.domain.model.ProfileDto;
import com.aitrack.server.domain.service.ProfileService;
import com.aitrack.server.infrastructure.config.AiTrackProperties;
import jakarta.servlet.http.HttpServletRequest;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.server.ResponseStatusException;

/**
 * Admin-gated endpoint for developer AI usage profiles.
 *
 * <ul>
 *   <li>GET /api/v1/ai-track/profiles/{tokenKey} — return profile for a given token</li>
 * </ul>
 *
 * Requires a valid {@code X-Admin-Key} header.
 * Returns 404 when no records and no active token are found for the given tokenKey.
 */
@RestController
@RequestMapping("/api/v1/ai-track")
@RequiredArgsConstructor
public class ProfileController {

    private final ProfileService profileService;
    private final AiTrackProperties props;

    /**
     * Returns the developer AI usage profile for the given token key.
     *
     * @param tokenKey 16-character token key
     */
    @GetMapping("/profiles/{tokenKey}")
    public ResponseEntity<ProfileDto> getProfile(
            HttpServletRequest httpRequest,
            @PathVariable String tokenKey
    ) {
        verifyAdminKey(httpRequest);

        return profileService.computeProfile(tokenKey)
                .map(ResponseEntity::ok)
                .orElseGet(() -> ResponseEntity.notFound().build());
    }

    // -------------------------------------------------------------------------
    // Admin key verification — mirrors EditSearchController pattern
    // -------------------------------------------------------------------------

    private void verifyAdminKey(HttpServletRequest request) {
        String configuredKey = props.getAdminKey();
        if (configuredKey == null || configuredKey.isBlank()) {
            throw new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE,
                "admin-key is not configured; set aitrack.admin-key before using this endpoint");
        }
        String provided = request.getHeader("X-Admin-Key");
        if (provided == null || !constantTimeEquals(configuredKey, provided.trim())) {
            throw new ResponseStatusException(HttpStatus.FORBIDDEN, "invalid X-Admin-Key");
        }
    }

    private static boolean constantTimeEquals(String a, String b) {
        if (a == null || b == null) return false;
        byte[] aBytes = a.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        byte[] bBytes = b.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        return java.security.MessageDigest.isEqual(aBytes, bBytes);
    }
}
