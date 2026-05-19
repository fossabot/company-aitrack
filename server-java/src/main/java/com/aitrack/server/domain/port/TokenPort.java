package com.aitrack.server.domain.port;

import com.aitrack.server.domain.model.TokenEntity;

import java.util.List;
import java.util.Optional;

/**
 * Driven-side persistence port for API tokens.
 *
 * <p>This is a pure domain interface (a secondary port of the hexagon).
 * The {@code adapter.db.TokenRepository} Spring Data interface provides the
 * concrete implementation; domain and application code depend only on this port.
 */
public interface TokenPort {

    /** Persists a token; returns the saved instance. */
    TokenEntity save(TokenEntity token);

    /** Returns every persisted token. */
    List<TokenEntity> findAll();

    /** Resolves an active token by its SHA-256 hash. */
    Optional<TokenEntity> findByTokenHashAndActiveTrue(String tokenHash);

    /** Resolves an active token by its token key. */
    Optional<TokenEntity> findByTokenKeyAndActiveTrue(String tokenKey);
}
