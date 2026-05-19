package com.aitrack.server.service;

import jakarta.persistence.EntityManager;
import jakarta.persistence.Query;
import lombok.RequiredArgsConstructor;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Semantic search service for edit records.
 * BM25 (ParadeDB) and ANN (pgvector) queries are only supported on PostgreSQL.
 * Returns 501 Not Implemented when running against H2.
 */
@Service
@RequiredArgsConstructor
public class EditSearchService {

    private final EntityManager entityManager;

    @Value("${spring.datasource.url:jdbc:h2:}")
    private String datasourceUrl;

    /** Returns true when the configured datasource is PostgreSQL. */
    boolean isPostgres() {
        return datasourceUrl != null && datasourceUrl.startsWith("jdbc:postgresql:");
    }

    /**
     * Full-text BM25 search via ParadeDB {@code |||} operator.
     *
     * @param query    ParadeDB query string
     * @param limit    max results (1-100)
     * @param tokenKey optional filter by token_key
     * @param repo     optional filter by repo
     * @return 200 with result list, or 501 if not PostgreSQL
     */
    @Transactional(readOnly = true)
    public ResponseEntity<Object> searchBm25(String query, int limit, String tokenKey, String repo) {
        if (!isPostgres()) {
            return notImplemented();
        }

        StringBuilder sql = new StringBuilder(
            "SELECT record_id, token_key, repo, file_path, diff_hunk, " +
            "ai_lines_added, ai_lines_removed, ts, " +
            "paradedb.score(id) AS score " +
            "FROM edit_records " +
            "WHERE diff_hunk ||| :query"
        );

        if (tokenKey != null && !tokenKey.isBlank()) {
            sql.append(" AND token_key = :tokenKey");
        }
        if (repo != null && !repo.isBlank()) {
            sql.append(" AND repo = :repo");
        }
        sql.append(" ORDER BY paradedb.score(id) DESC LIMIT :limit");

        Query nativeQuery = entityManager.createNativeQuery(sql.toString());
        nativeQuery.setParameter("query", query);
        nativeQuery.setParameter("limit", limit);
        if (tokenKey != null && !tokenKey.isBlank()) {
            nativeQuery.setParameter("tokenKey", tokenKey);
        }
        if (repo != null && !repo.isBlank()) {
            nativeQuery.setParameter("repo", repo);
        }

        @SuppressWarnings("unchecked")
        List<Object[]> rows = nativeQuery.getResultList();
        List<Map<String, Object>> results = mapBm25Rows(rows);
        return ResponseEntity.ok(Map.of("hits", results, "total", results.size()));
    }

    /**
     * Approximate nearest neighbour (ANN) search via pgvector {@code <=>} operator.
     *
     * @param embedding float array of length 384
     * @param limit     max results (1-50)
     * @param tokenKey  optional filter by token_key
     * @param repo      optional filter by repo
     * @return 200 with result list, or 501 if not PostgreSQL
     */
    @Transactional(readOnly = true)
    public ResponseEntity<Object> searchAnn(float[] embedding, int limit, String tokenKey, String repo) {
        if (!isPostgres()) {
            return notImplemented();
        }

        String embStr = toVectorLiteral(embedding);

        StringBuilder sql = new StringBuilder(
            "SELECT record_id, token_key, repo, file_path, diff_hunk, " +
            "ai_lines_added, ai_lines_removed, ts, " +
            "(embedding <=> CAST(:emb AS vector)) AS distance " +
            "FROM edit_records " +
            "WHERE embedding IS NOT NULL"
        );

        if (tokenKey != null && !tokenKey.isBlank()) {
            sql.append(" AND token_key = :tokenKey");
        }
        if (repo != null && !repo.isBlank()) {
            sql.append(" AND repo = :repo");
        }
        sql.append(" ORDER BY embedding <=> CAST(:emb AS vector) LIMIT :limit");

        Query nativeQuery = entityManager.createNativeQuery(sql.toString());
        nativeQuery.setParameter("emb", embStr);
        nativeQuery.setParameter("limit", limit);
        if (tokenKey != null && !tokenKey.isBlank()) {
            nativeQuery.setParameter("tokenKey", tokenKey);
        }
        if (repo != null && !repo.isBlank()) {
            nativeQuery.setParameter("repo", repo);
        }

        @SuppressWarnings("unchecked")
        List<Object[]> rows = nativeQuery.getResultList();
        List<Map<String, Object>> results = mapAnnRows(rows);
        return ResponseEntity.ok(Map.of("hits", results, "total", results.size()));
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    private static ResponseEntity<Object> notImplemented() {
        return ResponseEntity.status(HttpStatus.NOT_IMPLEMENTED)
            .body(Map.of("error", "semantic search requires PostgreSQL with ParadeDB/pgvector"));
    }

    /** Converts float[] to pgvector literal string, e.g. "[0.1,0.2,...]" */
    static String toVectorLiteral(float[] embedding) {
        StringBuilder sb = new StringBuilder("[");
        for (int i = 0; i < embedding.length; i++) {
            if (i > 0) sb.append(',');
            sb.append(embedding[i]);
        }
        sb.append(']');
        return sb.toString();
    }

    private static List<Map<String, Object>> mapBm25Rows(List<Object[]> rows) {
        List<Map<String, Object>> list = new ArrayList<>(rows.size());
        for (Object[] row : rows) {
            Map<String, Object> m = new LinkedHashMap<>();
            m.put("record_id",        row[0]);
            m.put("token_key",        row[1]);
            m.put("repo",             row[2]);
            m.put("file_path",        row[3]);
            m.put("diff_hunk",        row[4]);
            m.put("ai_lines_added",   row[5]);
            m.put("ai_lines_removed", row[6]);
            m.put("ts",               row[7]);
            m.put("score",            row[8]);
            list.add(m);
        }
        return list;
    }

    private static List<Map<String, Object>> mapAnnRows(List<Object[]> rows) {
        List<Map<String, Object>> list = new ArrayList<>(rows.size());
        for (Object[] row : rows) {
            Map<String, Object> m = new LinkedHashMap<>();
            m.put("record_id",        row[0]);
            m.put("token_key",        row[1]);
            m.put("repo",             row[2]);
            m.put("file_path",        row[3]);
            m.put("diff_hunk",        row[4]);
            m.put("ai_lines_added",   row[5]);
            m.put("ai_lines_removed", row[6]);
            m.put("ts",               row[7]);
            m.put("distance",         row[8]);
            list.add(m);
        }
        return list;
    }
}
