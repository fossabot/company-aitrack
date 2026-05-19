package com.aitrack.server.domain.model;

import java.util.List;

/**
 * Framework-agnostic pagination result for domain port return types.
 *
 * <p>Replaces {@code org.springframework.data.domain.Page} in port signatures so that
 * the domain layer has no dependency on Spring Data infrastructure types.
 *
 * @param <T> element type
 */
public record PageResult<T>(List<T> content, long totalElements) {}
