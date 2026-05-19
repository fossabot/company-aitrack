package com.aitrack.server.adapter.db;

import com.aitrack.server.domain.model.TokenEntity;
import com.aitrack.server.domain.port.TokenPort;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

import java.util.Optional;

/**
 * Spring Data JPA persistence adapter implementing {@link TokenPort}.
 */
@Repository
public interface TokenRepository extends JpaRepository<TokenEntity, Long>, TokenPort {

    // Most-specific re-declarations resolve the overload clash between
    // JpaRepository's generic CRUD signatures and the TokenPort port methods.
    @Override
    TokenEntity save(TokenEntity entity);

    @Override
    java.util.List<TokenEntity> findAll();

    @Override
    Optional<TokenEntity> findByTokenHashAndActiveTrue(String tokenHash);

    // Phase 3: used by ProfileService to look up token owner by tokenKey
    @Override
    Optional<TokenEntity> findByTokenKeyAndActiveTrue(String tokenKey);
}
