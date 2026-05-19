package com.aitrack.server.domain.model;

import jakarta.validation.constraints.NotBlank;
import lombok.Data;

@Data
public class CreateTokenRequest {
    @NotBlank private String owner;
    private String note;
}
