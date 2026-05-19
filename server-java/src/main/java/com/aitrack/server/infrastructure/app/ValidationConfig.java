package com.aitrack.server.infrastructure.app;

import com.aitrack.server.domain.service.ValidationPolicy;
import com.aitrack.server.infrastructure.config.AiTrackProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Composition root for the validation domain.
 *
 * <p>Maps the infrastructure-level {@code @ConfigurationProperties}
 * ({@link AiTrackProperties}) onto the pure domain value object
 * {@link ValidationPolicy}, which is then injected into
 * {@code domain.service.ValidationService}. This keeps the domain layer free
 * of any coupling to Spring config-binding types.
 */
@Configuration
public class ValidationConfig {

    /**
     * Produces the {@link ValidationPolicy} domain value object from the
     * externalised {@link AiTrackProperties} configuration.
     */
    @Bean
    public ValidationPolicy validationPolicy(AiTrackProperties props) {
        return new ValidationPolicy(
            props.getRateLimitPerHour(),
            props.getMaxAddedLines(),
            props.getRepoWhitelist().getUrls(),
            props.getRepoWhitelist().isEnforce()
        );
    }
}
