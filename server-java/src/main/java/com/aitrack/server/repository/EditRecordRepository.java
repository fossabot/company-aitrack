package com.aitrack.server.repository;

import com.aitrack.server.entity.EditRecordEntity;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;
import org.springframework.stereotype.Repository;

import java.time.Instant;
import java.util.List;

@Repository
public interface EditRecordRepository extends JpaRepository<EditRecordEntity, Long> {

    // Rate limit query by diffHunkHash stored separately — simpler approach: count by tokenKey+filePath+receivedAt
    @Query("SELECT COUNT(e) FROM EditRecordEntity e WHERE e.tokenKey = :tokenKey AND e.filePath = :filePath AND e.receivedAt >= :since")
    long countByTokenKeyAndFilePathSince(
        @Param("tokenKey") String tokenKey,
        @Param("filePath") String filePath,
        @Param("since") Instant since
    );

    Page<EditRecordEntity> findByTokenKey(String tokenKey, Pageable pageable);
    Page<EditRecordEntity> findByRepoUrl(String repoUrl, Pageable pageable);

    @Query("SELECT e FROM EditRecordEntity e WHERE (:tokenKey IS NULL OR e.tokenKey = :tokenKey) AND (:repoUrl IS NULL OR e.repoUrl = :repoUrl)")
    Page<EditRecordEntity> findByFilters(
        @Param("tokenKey") String tokenKey,
        @Param("repoUrl") String repoUrl,
        Pageable pageable
    );

    // Stats aggregation queries
    @Query("SELECT e.tokenKey, COUNT(e), SUM(e.addedLines), SUM(e.removedLines), " +
           "SUM(CASE WHEN e.status = 'ACCEPTED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'FLAGGED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'REJECTED' THEN 1 ELSE 0 END), " +
           "MAX(e.receivedAt) FROM EditRecordEntity e GROUP BY e.tokenKey")
    java.util.List<Object[]> aggregateByTokenKey();

    @Query("SELECT e.repoUrl, COUNT(e), SUM(e.addedLines), SUM(e.removedLines), " +
           "SUM(CASE WHEN e.status = 'ACCEPTED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'FLAGGED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'REJECTED' THEN 1 ELSE 0 END), " +
           "MAX(e.receivedAt) FROM EditRecordEntity e GROUP BY e.repoUrl")
    java.util.List<Object[]> aggregateByRepo();

    @Query("SELECT e.deviceId, COUNT(e), SUM(e.addedLines), SUM(e.removedLines), " +
           "SUM(CASE WHEN e.status = 'ACCEPTED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'FLAGGED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'REJECTED' THEN 1 ELSE 0 END), " +
           "MAX(e.receivedAt) FROM EditRecordEntity e GROUP BY e.deviceId")
    java.util.List<Object[]> aggregateByDevice();

    @Query("SELECT e.hostname, COUNT(e), SUM(e.addedLines), SUM(e.removedLines), " +
           "SUM(CASE WHEN e.status = 'ACCEPTED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'FLAGGED' THEN 1 ELSE 0 END), " +
           "SUM(CASE WHEN e.status = 'REJECTED' THEN 1 ELSE 0 END), " +
           "MAX(e.receivedAt) FROM EditRecordEntity e GROUP BY e.hostname")
    java.util.List<Object[]> aggregateByHostname();

    // BM25 full-text search via ParadeDB — only functional on the postgres profile.
    // Will fail if invoked against H2; not wired to any controller yet (Phase DB-2).
    @Query(value = "SELECT * FROM edit_records WHERE diff_hunk ||| :query ORDER BY paradedb.score(id) DESC LIMIT :limit", nativeQuery = true)
    List<EditRecordEntity> searchBm25(@Param("query") String query, @Param("limit") int limit);

    // Phase 3: used by ProfileService to load all non-rejected records for a token
    List<EditRecordEntity> findByTokenKeyAndStatusNot(String tokenKey, EditRecordEntity.RecordStatus status);
}
