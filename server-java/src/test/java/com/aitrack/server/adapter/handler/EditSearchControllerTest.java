package com.aitrack.server.adapter.handler;

import com.aitrack.server.application.EditSearchService;
import com.aitrack.server.infrastructure.config.AiTrackProperties;
import com.aitrack.server.infrastructure.config.AiTrackServerApplication;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.test.context.ContextConfiguration;
import org.springframework.test.context.TestPropertySource;
import org.springframework.test.web.servlet.MockMvc;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

/**
 * Slice tests for {@link EditSearchController}.
 * Uses {@code @WebMvcTest} with a mocked {@link EditSearchService} so that
 * no real database or EntityManager is needed.
 */
@WebMvcTest(EditSearchController.class)
@ContextConfiguration(classes = AiTrackServerApplication.class)
@TestPropertySource(properties = {
    "aitrack.admin-key=test-admin-key-do-not-use-in-prod",
    "spring.datasource.url=jdbc:h2:mem:testdb"
})
class EditSearchControllerTest {

    private static final String ADMIN_KEY   = "test-admin-key-do-not-use-in-prod";
    private static final String SEARCH_URL  = "/api/v1/ai-track/edits/search";
    private static final String SIMILAR_URL = "/api/v1/ai-track/edits/similar";

    @Autowired MockMvc mockMvc;
    @Autowired ObjectMapper objectMapper;

    @MockBean EditSearchService editSearchService;
    // AiTrackProperties must be in the context; WebMvcTest auto-discovers @Component beans
    // but ConfigurationProperties classes need explicit wiring — provide via @MockBean or
    // a @TestConfiguration.  Since we set properties via @TestPropertySource we need the
    // real bean; expose it via @Import is cumbersome, so we mock it and stub getAdminKey().
    @MockBean AiTrackProperties aiTrackProperties;

    // -------------------------------------------------------------------------
    // GET /edits/search — BM25
    // -------------------------------------------------------------------------

    /** Test 1: missing X-Admin-Key → 403 */
    @Test
    void search_noAdminKey_403() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        mockMvc.perform(get(SEARCH_URL).param("q", "hello"))
               .andExpect(status().isForbidden());
    }

    /** Test 2: q param absent → 400 */
    @Test
    void search_missingQ_400() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        // Spring MVC raises 400 automatically for a required @RequestParam that is absent
        mockMvc.perform(get(SEARCH_URL)
                .header("X-Admin-Key", ADMIN_KEY))
               .andExpect(status().isBadRequest());
    }

    /** Test 3: valid request → 200 with hits */
    @Test
    void search_valid_200() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        Map<String, Object> mockResponse = Map.of(
            "hits", List.of(Map.of("record_id", "abc", "score", 1.23)),
            "total", 1
        );
        when(editSearchService.searchBm25(eq("hello"), anyInt(), any(), any()))
            .thenReturn(ResponseEntity.ok(mockResponse));

        mockMvc.perform(get(SEARCH_URL)
                .header("X-Admin-Key", ADMIN_KEY)
                .param("q", "hello"))
               .andExpect(status().isOk())
               .andExpect(jsonPath("$.total").value(1))
               .andExpect(jsonPath("$.hits").isArray());
    }

    /** Test 4: service returns 501 (H2 mode) → controller propagates 501 */
    @Test
    void search_h2Mode_501() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        when(editSearchService.searchBm25(any(), anyInt(), any(), any()))
            .thenReturn(ResponseEntity.status(HttpStatus.NOT_IMPLEMENTED)
                .body(Map.of("error", "not implemented")));

        mockMvc.perform(get(SEARCH_URL)
                .header("X-Admin-Key", ADMIN_KEY)
                .param("q", "hello"))
               .andExpect(status().isNotImplemented());
    }

    // -------------------------------------------------------------------------
    // POST /edits/similar — ANN
    // -------------------------------------------------------------------------

    /** Test 5: missing X-Admin-Key → 403 */
    @Test
    void similar_noAdminKey_403() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        String body = objectMapper.writeValueAsString(Map.of("embedding", validEmbedding()));
        mockMvc.perform(post(SIMILAR_URL)
                .contentType(MediaType.APPLICATION_JSON)
                .content(body))
               .andExpect(status().isForbidden());
    }

    /** Test 6: embedding has wrong dimension → 400 */
    @Test
    void similar_wrongDimension_400() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        // Only 10 values instead of 384
        List<Float> shortEmb = new ArrayList<>();
        for (int i = 0; i < 10; i++) shortEmb.add(0.1f);
        String body = objectMapper.writeValueAsString(Map.of("embedding", shortEmb));

        mockMvc.perform(post(SIMILAR_URL)
                .header("X-Admin-Key", ADMIN_KEY)
                .contentType(MediaType.APPLICATION_JSON)
                .content(body))
               .andExpect(status().isBadRequest());
    }

    /** Test 7: valid request → 200 with hits */
    @Test
    void similar_valid_200() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        Map<String, Object> mockResponse = Map.of(
            "hits", List.of(Map.of("record_id", "xyz", "distance", 0.05)),
            "total", 1
        );
        when(editSearchService.searchAnn(any(float[].class), anyInt(), any(), any()))
            .thenReturn(ResponseEntity.ok(mockResponse));

        String body = objectMapper.writeValueAsString(Map.of("embedding", validEmbedding()));
        mockMvc.perform(post(SIMILAR_URL)
                .header("X-Admin-Key", ADMIN_KEY)
                .contentType(MediaType.APPLICATION_JSON)
                .content(body))
               .andExpect(status().isOk())
               .andExpect(jsonPath("$.total").value(1))
               .andExpect(jsonPath("$.hits").isArray());
    }

    /** Test 8: service returns 501 (H2 mode) → controller propagates 501 */
    @Test
    void similar_h2Mode_501() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        when(editSearchService.searchAnn(any(float[].class), anyInt(), any(), any()))
            .thenReturn(ResponseEntity.status(HttpStatus.NOT_IMPLEMENTED)
                .body(Map.of("error", "not implemented")));

        String body = objectMapper.writeValueAsString(Map.of("embedding", validEmbedding()));
        mockMvc.perform(post(SIMILAR_URL)
                .header("X-Admin-Key", ADMIN_KEY)
                .contentType(MediaType.APPLICATION_JSON)
                .content(body))
               .andExpect(status().isNotImplemented());
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    /** Returns a 384-element list of floats (all 0.1f). */
    private static List<Float> validEmbedding() {
        List<Float> emb = new ArrayList<>(384);
        for (int i = 0; i < 384; i++) emb.add(0.1f);
        return emb;
    }
}
