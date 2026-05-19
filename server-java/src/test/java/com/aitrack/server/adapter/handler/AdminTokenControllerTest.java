package com.aitrack.server.adapter.handler;

import com.aitrack.server.infrastructure.config.AiTrackServerApplication;
import com.aitrack.server.testkit.CreateTokenRequestFactory;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.annotation.DirtiesContext;
import org.springframework.test.web.servlet.MockMvc;

import static org.hamcrest.Matchers.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

/**
 * Tests for POST /admin/tokens.
 * The test application.yml sets aitrack.admin-key = "test-admin-key-do-not-use-in-prod".
 */
@SpringBootTest(classes = AiTrackServerApplication.class)
@AutoConfigureMockMvc
@DirtiesContext(classMode = DirtiesContext.ClassMode.AFTER_EACH_TEST_METHOD)
class AdminTokenControllerTest {

    private static final String ADMIN_KEY = "test-admin-key-do-not-use-in-prod";

    @Autowired MockMvc mockMvc;
    @Autowired ObjectMapper objectMapper;

    private org.springframework.test.web.servlet.ResultActions createToken(String adminKey, Object body) throws Exception {
        var request = post("/admin/tokens")
                .contentType(MediaType.APPLICATION_JSON)
                .content(objectMapper.writeValueAsBytes(body));
        if (adminKey != null) {
            request = request.header("X-Admin-Key", adminKey);
        }
        return mockMvc.perform(request);
    }

    @Test
    void validRequest_200_returnsCredential() throws Exception {
        createToken(ADMIN_KEY, CreateTokenRequestFactory.build())
                .andExpect(status().isOk())
                // credential = "<token>-<hmac_secret>"; token starts with "aitrack_" and has no '-'
                .andExpect(jsonPath("$.credential").value(startsWith("aitrack_")))
                .andExpect(jsonPath("$.credential").value(containsString("-")))
                .andExpect(jsonPath("$.token_key").isNotEmpty())
                // v1.2: token and hmac_secret must NOT be separate top-level fields
                .andExpect(jsonPath("$.token").doesNotExist())
                .andExpect(jsonPath("$.hmac_secret").doesNotExist());
    }

    @Test
    void wrongAdminKey_403() throws Exception {
        createToken("wrong-key", CreateTokenRequestFactory.build())
                .andExpect(status().isForbidden());
    }

    @Test
    void missingAdminKey_403() throws Exception {
        createToken(null, CreateTokenRequestFactory.build())
                .andExpect(status().isForbidden());
    }

    @Test
    void blankAdminKey_403() throws Exception {
        createToken("   ", CreateTokenRequestFactory.build())
                .andExpect(status().isForbidden());
    }

    @Test
    void missingOwnerField_400() throws Exception {
        // owner is @NotBlank — missing it triggers Bean Validation 400
        String body = "{\"note\":\"test\"}";
        mockMvc.perform(post("/admin/tokens")
                .header("X-Admin-Key", ADMIN_KEY)
                .contentType(MediaType.APPLICATION_JSON)
                .content(body))
                .andExpect(status().isBadRequest());
    }

    @Test
    void blankOwner_400() throws Exception {
        String body = "{\"owner\":\"\",\"note\":\"test\"}";
        mockMvc.perform(post("/admin/tokens")
                .header("X-Admin-Key", ADMIN_KEY)
                .contentType(MediaType.APPLICATION_JSON)
                .content(body))
                .andExpect(status().isBadRequest());
    }

    @Test
    void withNote_200_noteIsOptional() throws Exception {
        // note is optional
        createToken(ADMIN_KEY, CreateTokenRequestFactory.with(r -> r.setNote(null)))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.credential").value(startsWith("aitrack_")));
    }

    @Test
    void credential_splitOnFirstDash_yieldsTokenAndHmacSecret() throws Exception {
        String body = createToken(ADMIN_KEY, CreateTokenRequestFactory.build())
                .andExpect(status().isOk())
                .andReturn().getResponse().getContentAsString();

        com.fasterxml.jackson.databind.JsonNode json = objectMapper.readTree(body);
        String credential = json.get("credential").asText();

        // Split on first '-' only
        int dashIdx = credential.indexOf('-');
        org.assertj.core.api.Assertions.assertThat(dashIdx).isPositive();
        String token = credential.substring(0, dashIdx);
        String hmacSecret = credential.substring(dashIdx + 1);

        org.assertj.core.api.Assertions.assertThat(token).startsWith("aitrack_");
        // token must not contain '-'
        org.assertj.core.api.Assertions.assertThat(token).doesNotContain("-");
        // hmacSecret is 64-char hex (32 random bytes)
        org.assertj.core.api.Assertions.assertThat(hmacSecret).matches("[0-9a-f]{64}");
    }

    @Test
    void twoTokens_haveDifferentValues() throws Exception {
        String res1 = mockMvc.perform(post("/admin/tokens")
                .header("X-Admin-Key", ADMIN_KEY)
                .contentType(MediaType.APPLICATION_JSON)
                .content(objectMapper.writeValueAsBytes(CreateTokenRequestFactory.build())))
                .andExpect(status().isOk())
                .andReturn().getResponse().getContentAsString();

        String res2 = mockMvc.perform(post("/admin/tokens")
                .header("X-Admin-Key", ADMIN_KEY)
                .contentType(MediaType.APPLICATION_JSON)
                .content(objectMapper.writeValueAsBytes(CreateTokenRequestFactory.build())))
                .andExpect(status().isOk())
                .andReturn().getResponse().getContentAsString();

        // Each token must be unique
        org.assertj.core.api.Assertions.assertThat(res1).isNotEqualTo(res2);
    }
}
