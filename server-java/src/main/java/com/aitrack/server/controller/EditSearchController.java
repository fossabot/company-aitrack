package com.aitrack.server.controller;

import com.aitrack.server.config.AiTrackProperties;
import com.aitrack.server.service.EditSearchService;
import jakarta.servlet.http.HttpServletRequest;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.server.ResponseStatusException;

import java.util.List;
import java.util.Map;

/**
 * Admin-gated endpoints for semantic search over edit records.
 *
 * <ul>
 *   <li>GET  /api/v1/ai-track/edits/search — BM25 full-text search (ParadeDB)</li>
 *   <li>POST /api/v1/ai-track/edits/similar — ANN vector search (pgvector)</li>
 * </ul>
 *
 * Both endpoints require a valid {@code X-Admin-Key} header.
 * Both return 501 when the server is running against H2 (non-PostgreSQL).
 */
@RestController
@RequestMapping("/api/v1/ai-track")
@RequiredArgsConstructor
public class EditSearchController {

    private final EditSearchService editSearchService;
    private final AiTrackProperties props;

    /**
     * BM25 full-text search.
     *
     * @param q         required query string (ParadeDB syntax)
     * @param limit     max results, 1-100, default 20
     * @param token_key optional filter by token_key
     * @param repo      optional filter by repo
     */
    @GetMapping("/edits/search")
    public ResponseEntity<Object> searchBm25(
        HttpServletRequest httpRequest,
        @RequestParam String q,
        @RequestParam(defaultValue = "20") int limit,
        @RequestParam(required = false) String token_key,
        @RequestParam(required = false) String repo
    ) {
        verifyAdminKey(httpRequest);

        if (q == null || q.isBlank()) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "query parameter 'q' is required");
        }

        int clampedLimit = Math.min(100, Math.max(1, limit));
        return editSearchService.searchBm25(q, clampedLimit, token_key, repo);
    }

    /**
     * ANN (approximate nearest neighbour) vector search.
     * Body: {@code {embedding: float[], limit: int, token_key: string, repo: string}}
     * Embedding must have exactly 384 dimensions.
     */
    @PostMapping("/edits/similar")
    public ResponseEntity<Object> searchAnn(
        HttpServletRequest httpRequest,
        @RequestBody Map<String, Object> body
    ) {
        verifyAdminKey(httpRequest);

        // Parse embedding
        Object rawEmb = body.get("embedding");
        if (rawEmb == null) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "'embedding' is required");
        }
        if (!(rawEmb instanceof List)) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "'embedding' must be an array");
        }
        @SuppressWarnings("unchecked")
        List<Number> embList = (List<Number>) rawEmb;
        if (embList.size() != 384) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST,
                "embedding must have exactly 384 dimensions, got " + embList.size());
        }
        float[] embedding = new float[384];
        for (int i = 0; i < 384; i++) {
            embedding[i] = embList.get(i).floatValue();
        }

        // Parse limit
        int limit = 20;
        if (body.containsKey("limit") && body.get("limit") instanceof Number n) {
            limit = n.intValue();
        }
        int clampedLimit = Math.min(50, Math.max(1, limit));

        // Optional filters
        String tokenKey = body.containsKey("token_key") ? (String) body.get("token_key") : null;
        String repo     = body.containsKey("repo")      ? (String) body.get("repo")      : null;

        return editSearchService.searchAnn(embedding, clampedLimit, tokenKey, repo);
    }

    // -------------------------------------------------------------------------
    // Admin key verification — mirrors AdminTokenController pattern
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
