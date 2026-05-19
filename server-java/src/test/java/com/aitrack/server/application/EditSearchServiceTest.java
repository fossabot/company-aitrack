package com.aitrack.server.application;

import jakarta.persistence.EntityManager;
import jakarta.persistence.Query;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.test.util.ReflectionTestUtils;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link EditSearchService}.
 *
 * Uses Mockito's {@code @ExtendWith} so no Spring context is needed.
 * {@code ReflectionTestUtils} is used to inject the {@code datasourceUrl}
 * field that would normally come from {@code @Value}.
 */
@ExtendWith(MockitoExtension.class)
class EditSearchServiceTest {

    @Mock
    private EntityManager entityManager;

    @Mock
    private Query mockQuery;

    @InjectMocks
    private EditSearchService service;

    // -------------------------------------------------------------------------
    // isPostgres()
    // -------------------------------------------------------------------------

    @Test
    void isPostgres_h2_returnsFalse() {
        setDatasourceUrl("jdbc:h2:mem:testdb");
        assertThat(service.isPostgres()).isFalse();
    }

    @Test
    void isPostgres_postgresql_returnsTrue() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        assertThat(service.isPostgres()).isTrue();
    }

    @Test
    void isPostgres_nullUrl_returnsFalse() {
        setDatasourceUrl(null);
        assertThat(service.isPostgres()).isFalse();
    }

    @Test
    void isPostgres_emptyUrl_returnsFalse() {
        setDatasourceUrl("");
        assertThat(service.isPostgres()).isFalse();
    }

    // -------------------------------------------------------------------------
    // searchBm25() — H2 mode (501)
    // -------------------------------------------------------------------------

    @Test
    void searchBm25_h2Mode_returns501() {
        setDatasourceUrl("jdbc:h2:mem:testdb");

        ResponseEntity<Object> r = service.searchBm25("test query", 10, null, null);

        assertThat(r.getStatusCode().value()).isEqualTo(HttpStatus.NOT_IMPLEMENTED.value());
        assertThat(r.getBody()).isInstanceOf(Map.class);
        @SuppressWarnings("unchecked")
        Map<String, Object> body = (Map<String, Object>) r.getBody();
        assertThat(body).containsKey("error");
    }

    @Test
    void searchBm25_h2Mode_noEntityManagerInteraction() {
        setDatasourceUrl("jdbc:h2:mem:testdb");

        service.searchBm25("test", 10, "tok", "myrepo");

        verifyNoInteractions(entityManager);
    }

    // -------------------------------------------------------------------------
    // searchAnn() — H2 mode (501)
    // -------------------------------------------------------------------------

    @Test
    void searchAnn_h2Mode_returns501() {
        setDatasourceUrl("jdbc:h2:mem:testdb");
        float[] emb = new float[384];

        ResponseEntity<Object> r = service.searchAnn(emb, 10, null, null);

        assertThat(r.getStatusCode().value()).isEqualTo(HttpStatus.NOT_IMPLEMENTED.value());
        assertThat(r.getBody()).isInstanceOf(Map.class);
        @SuppressWarnings("unchecked")
        Map<String, Object> body = (Map<String, Object>) r.getBody();
        assertThat(body).containsKey("error");
    }

    @Test
    void searchAnn_h2Mode_noEntityManagerInteraction() {
        setDatasourceUrl("jdbc:h2:mem:testdb");

        service.searchAnn(new float[384], 10, "tok", "myrepo");

        verifyNoInteractions(entityManager);
    }

    // -------------------------------------------------------------------------
    // searchBm25() — Postgres mode (200 paths)
    // -------------------------------------------------------------------------

    @Test
    void searchBm25_postgresMode_emptyResult_returns200WithEmptyHits() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);
        when(mockQuery.getResultList()).thenReturn(List.of());

        ResponseEntity<Object> r = service.searchBm25("test", 10, null, null);

        assertThat(r.getStatusCode().value()).isEqualTo(200);
        @SuppressWarnings("unchecked")
        Map<String, Object> body = (Map<String, Object>) r.getBody();
        assertThat(body).containsKey("hits");
        assertThat(body).containsEntry("total", 0);
        @SuppressWarnings("unchecked")
        List<?> hits = (List<?>) body.get("hits");
        assertThat(hits).isEmpty();
    }

    @Test
    void searchBm25_postgresMode_withTokenKeyAndRepo_passesFilters() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);
        when(mockQuery.getResultList()).thenReturn(List.of());

        ResponseEntity<Object> r = service.searchBm25("query", 5, "myToken", "myRepo");

        assertThat(r.getStatusCode().value()).isEqualTo(200);
        // verify tokenKey and repo params were set
        verify(mockQuery).setParameter("tokenKey", "myToken");
        verify(mockQuery).setParameter("repo", "myRepo");
    }

    @Test
    void searchBm25_postgresMode_withResults_mapsRowsCorrectly() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);

        // Simulate one BM25 result row:
        // record_id, token_key, repo, file_path, diff_hunk, ai_lines_added, ai_lines_removed, ts, score
        Object[] row = {"rec1", "tok1", "repo1", "path/to/file.java",
                        "@@ -1,1 +1,1 @@ ...", 2, 1, "2024-01-01T00:00:00Z", 0.95f};
        List<Object[]> rows = Collections.singletonList(row);
        when(mockQuery.getResultList()).thenReturn(rows);

        ResponseEntity<Object> r = service.searchBm25("query", 10, null, null);

        assertThat(r.getStatusCode().value()).isEqualTo(200);
        @SuppressWarnings("unchecked")
        Map<String, Object> body = (Map<String, Object>) r.getBody();
        assertThat(body).containsEntry("total", 1);
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> hits = (List<Map<String, Object>>) body.get("hits");
        assertThat(hits).hasSize(1);
        assertThat(hits.get(0)).containsEntry("record_id", "rec1");
        assertThat(hits.get(0)).containsKey("score");
    }

    @Test
    void searchBm25_postgresMode_nullTokenKeyAndRepo_noFilterParams() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);
        when(mockQuery.getResultList()).thenReturn(List.of());

        service.searchBm25("query", 10, null, null);

        // tokenKey and repo params must NOT be set when null
        verify(mockQuery, never()).setParameter(eq("tokenKey"), any());
        verify(mockQuery, never()).setParameter(eq("repo"), any());
    }

    // -------------------------------------------------------------------------
    // searchAnn() — Postgres mode (200 paths)
    // -------------------------------------------------------------------------

    @Test
    void searchAnn_postgresMode_emptyResult_returns200WithEmptyHits() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);
        when(mockQuery.getResultList()).thenReturn(List.of());

        float[] emb = new float[384];
        ResponseEntity<Object> r = service.searchAnn(emb, 10, null, null);

        assertThat(r.getStatusCode().value()).isEqualTo(200);
        @SuppressWarnings("unchecked")
        Map<String, Object> body = (Map<String, Object>) r.getBody();
        assertThat(body).containsKey("hits");
        assertThat(body).containsEntry("total", 0);
    }

    @Test
    void searchAnn_postgresMode_withTokenKeyAndRepo_passesFilters() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);
        when(mockQuery.getResultList()).thenReturn(List.of());

        ResponseEntity<Object> r = service.searchAnn(new float[384], 5, "myToken", "myRepo");

        assertThat(r.getStatusCode().value()).isEqualTo(200);
        verify(mockQuery).setParameter("tokenKey", "myToken");
        verify(mockQuery).setParameter("repo", "myRepo");
    }

    @Test
    void searchAnn_postgresMode_withResults_mapsRowsCorrectly() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);

        // record_id, token_key, repo, file_path, diff_hunk, ai_lines_added, ai_lines_removed, ts, distance
        Object[] row = {"rec2", "tok2", "repo2", "src/Foo.java",
                        "@@ -1,1 +1,1 @@", 1, 0, "2024-02-01T00:00:00Z", 0.12f};
        List<Object[]> rows = Collections.singletonList(row);
        when(mockQuery.getResultList()).thenReturn(rows);

        float[] emb = new float[384];
        ResponseEntity<Object> r = service.searchAnn(emb, 10, null, null);

        assertThat(r.getStatusCode().value()).isEqualTo(200);
        @SuppressWarnings("unchecked")
        Map<String, Object> body = (Map<String, Object>) r.getBody();
        assertThat(body).containsEntry("total", 1);
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> hits = (List<Map<String, Object>>) body.get("hits");
        assertThat(hits).hasSize(1);
        assertThat(hits.get(0)).containsEntry("record_id", "rec2");
        assertThat(hits.get(0)).containsKey("distance");
    }

    @Test
    void searchAnn_postgresMode_embeddingConvertedToVectorLiteral() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);
        when(mockQuery.getResultList()).thenReturn(List.of());

        float[] emb = {0.1f, 0.2f, 0.3f};
        service.searchAnn(emb, 5, null, null);

        // The embedding should be passed as a vector literal string like "[0.1,0.2,0.3]"
        verify(mockQuery).setParameter(eq("emb"), argThat(arg ->
            arg instanceof String s && s.startsWith("[") && s.endsWith("]") && s.contains(",")
        ));
    }

    @Test
    void searchAnn_postgresMode_nullTokenKeyAndRepo_noFilterParams() {
        setDatasourceUrl("jdbc:postgresql://localhost:5432/aitrack");
        when(entityManager.createNativeQuery(anyString())).thenReturn(mockQuery);
        when(mockQuery.setParameter(anyString(), any())).thenReturn(mockQuery);
        when(mockQuery.getResultList()).thenReturn(List.of());

        service.searchAnn(new float[384], 10, null, null);

        verify(mockQuery, never()).setParameter(eq("tokenKey"), any());
        verify(mockQuery, never()).setParameter(eq("repo"), any());
    }

    // -------------------------------------------------------------------------
    // toVectorLiteral() — static helper
    // -------------------------------------------------------------------------

    @Test
    void toVectorLiteral_threeElements_correctFormat() {
        float[] emb = {0.1f, -0.5f, 1.0f};
        String lit = EditSearchService.toVectorLiteral(emb);
        assertThat(lit).startsWith("[");
        assertThat(lit).endsWith("]");
        assertThat(lit).contains(",");
        // Exactly 2 commas for 3 values
        assertThat(lit.chars().filter(c -> c == ',').count()).isEqualTo(2);
    }

    @Test
    void toVectorLiteral_singleElement_noComa() {
        float[] emb = {0.42f};
        String lit = EditSearchService.toVectorLiteral(emb);
        assertThat(lit).startsWith("[");
        assertThat(lit).endsWith("]");
        assertThat(lit).doesNotContain(",");
        assertThat(lit).contains("0.42");
    }

    @Test
    void toVectorLiteral_emptyArray_emptyBrackets() {
        float[] emb = {};
        String lit = EditSearchService.toVectorLiteral(emb);
        assertThat(lit).isEqualTo("[]");
    }

    @Test
    void toVectorLiteral_negativeValues_preservedInOutput() {
        float[] emb = {-1.0f, -0.001f};
        String lit = EditSearchService.toVectorLiteral(emb);
        assertThat(lit).contains("-1.0");
        assertThat(lit).contains("-0.001");
    }

    @Test
    void toVectorLiteral_384Elements_correctCommaCount() {
        float[] emb = new float[384];
        for (int i = 0; i < emb.length; i++) emb[i] = i * 0.001f;
        String lit = EditSearchService.toVectorLiteral(emb);
        assertThat(lit).startsWith("[");
        assertThat(lit).endsWith("]");
        assertThat(lit.chars().filter(c -> c == ',').count()).isEqualTo(383);
    }

    // -------------------------------------------------------------------------
    // Helper
    // -------------------------------------------------------------------------

    private void setDatasourceUrl(String url) {
        ReflectionTestUtils.setField(service, "datasourceUrl", url);
    }
}
