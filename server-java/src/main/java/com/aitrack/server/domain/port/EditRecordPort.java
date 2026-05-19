package com.aitrack.server.domain.port;

import com.aitrack.server.domain.model.EditRecordEntity;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;

import java.time.Instant;
import java.util.List;

/**
 * Driven-side persistence port for edit records.
 *
 * <p>This is a pure domain interface (a secondary port of the hexagon).
 * The {@code adapter.db.EditRecordRepository} Spring Data interface provides the
 * concrete implementation; domain and application code depend only on this port.
 *
 * <p>{@link Page}/{@link Pageable} are framework-neutral pagination abstractions
 * and are kept in the port signature so the query contract is preserved verbatim.
 */
public interface EditRecordPort {

    /** Persists a single edit record; returns the saved instance. */
    EditRecordEntity save(EditRecordEntity record);

    /** Counts records for a token+file within a rolling window (rate limiting). */
    long countByTokenKeyAndFilePathSince(String tokenKey, String filePath, Instant since);

    /** Returns a page of records optionally filtered by token key and repo URL. */
    Page<EditRecordEntity> findByFilters(String tokenKey, String repoUrl, Pageable pageable);

    /** Loads all non-rejected records for a token, excluding the given status. */
    List<EditRecordEntity> findByTokenKeyAndStatusNot(String tokenKey, EditRecordEntity.RecordStatus status);

    /** Aggregates stats grouped by token key. */
    List<Object[]> aggregateByTokenKey();

    /** Aggregates stats grouped by repo URL. */
    List<Object[]> aggregateByRepo();

    /** Aggregates stats grouped by device ID. */
    List<Object[]> aggregateByDevice();

    /** Aggregates stats grouped by hostname. */
    List<Object[]> aggregateByHostname();
}
