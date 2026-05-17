package com.aitrack.server.dto;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;

@Data
@AllArgsConstructor
public class CreateTokenResponse {
    /** Combined credential: "<token>-<hmac_secret>". Split on first '-' to recover the two parts. */
    private String credential;
    @JsonProperty("token_key") private String tokenKey;
}
