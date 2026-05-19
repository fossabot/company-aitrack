package com.aitrack.server.controller;

import com.aitrack.server.config.AiTrackProperties;
import com.aitrack.server.dto.ProfileDto;
import com.aitrack.server.service.ProfileService;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.test.context.TestPropertySource;
import org.springframework.test.web.servlet.MockMvc;

import java.util.Optional;

import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

/**
 * Slice tests for {@link ProfileController}.
 * Uses {@code @WebMvcTest} with a mocked {@link ProfileService} so that
 * no real database or EntityManager is needed.
 */
@WebMvcTest(ProfileController.class)
@TestPropertySource(properties = {
    "aitrack.admin-key=test-admin-key-do-not-use-in-prod",
    "spring.datasource.url=jdbc:h2:mem:testdb"
})
class ProfileControllerTest {

    private static final String ADMIN_KEY   = "test-admin-key-do-not-use-in-prod";
    private static final String PROFILE_URL = "/api/v1/ai-track/profiles/abc123";
    private static final String TOKEN_KEY   = "abc123";

    @Autowired MockMvc mockMvc;
    @Autowired ObjectMapper objectMapper;

    @MockBean ProfileService profileService;
    @MockBean AiTrackProperties aiTrackProperties;

    // -------------------------------------------------------------------------
    // Test 1: missing X-Admin-Key → 403
    // -------------------------------------------------------------------------

    @Test
    void profile_noAdminKey_403() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        mockMvc.perform(get(PROFILE_URL))
               .andExpect(status().isForbidden());
    }

    // -------------------------------------------------------------------------
    // Test 2: wrong X-Admin-Key value → 403
    // -------------------------------------------------------------------------

    @Test
    void profile_wrongAdminKey_403() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        mockMvc.perform(get(PROFILE_URL)
                .header("X-Admin-Key", "wrong-key"))
               .andExpect(status().isForbidden());
    }

    // -------------------------------------------------------------------------
    // Test 3: valid key but service returns empty → 404
    // -------------------------------------------------------------------------

    @Test
    void profile_tokenNotFound_404() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);
        when(profileService.computeProfile(eq(TOKEN_KEY))).thenReturn(Optional.empty());

        mockMvc.perform(get(PROFILE_URL)
                .header("X-Admin-Key", ADMIN_KEY))
               .andExpect(status().isNotFound());
    }

    // -------------------------------------------------------------------------
    // Test 4: valid key, service returns valid ProfileDto → 200 + check token_key
    // -------------------------------------------------------------------------

    @Test
    void profile_validToken_200() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        ProfileDto profile = new ProfileDto();
        profile.setTokenKey(TOKEN_KEY);
        profile.setOwner("alice");
        profile.setTotalEdits(42);
        profile.setTotalAddedLines(100);
        profile.setTotalRemovedLines(20);
        profile.setGeneratedAt("2026-05-19T00:00:00Z");

        when(profileService.computeProfile(eq(TOKEN_KEY))).thenReturn(Optional.of(profile));

        mockMvc.perform(get(PROFILE_URL)
                .header("X-Admin-Key", ADMIN_KEY))
               .andExpect(status().isOk())
               .andExpect(jsonPath("$.token_key").value(TOKEN_KEY))
               .andExpect(jsonPath("$.owner").value("alice"))
               .andExpect(jsonPath("$.total_edits").value(42));
    }

    // -------------------------------------------------------------------------
    // Test 5: valid key, empty-but-valid profile (zero totals, null sub-objects) → 200
    // -------------------------------------------------------------------------

    @Test
    void profile_emptyProfile_200() throws Exception {
        when(aiTrackProperties.getAdminKey()).thenReturn(ADMIN_KEY);

        ProfileDto profile = new ProfileDto();
        profile.setTokenKey(TOKEN_KEY);
        profile.setTotalEdits(0);
        profile.setTotalAddedLines(0);
        profile.setTotalRemovedLines(0);
        profile.setGeneratedAt("2026-05-19T00:00:00Z");
        // frequency, depth, scenarios, tools all null

        when(profileService.computeProfile(eq(TOKEN_KEY))).thenReturn(Optional.of(profile));

        mockMvc.perform(get(PROFILE_URL)
                .header("X-Admin-Key", ADMIN_KEY))
               .andExpect(status().isOk())
               .andExpect(jsonPath("$.token_key").value(TOKEN_KEY))
               .andExpect(jsonPath("$.total_edits").value(0))
               .andExpect(jsonPath("$.frequency").doesNotExist())
               .andExpect(jsonPath("$.depth").doesNotExist());
    }
}
