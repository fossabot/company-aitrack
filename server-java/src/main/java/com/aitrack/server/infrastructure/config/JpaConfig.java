package com.aitrack.server.infrastructure.config;

import org.springframework.boot.autoconfigure.domain.EntityScan;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.jpa.repository.config.EnableJpaRepositories;

/**
 * JPA wiring for the hexagon's persistence side.
 *
 * <p>Entity scanning and Spring Data repository scanning are declared here
 * — separately from {@code AiTrackServerApplication} — so that web-layer
 * slice tests ({@code @WebMvcTest}) do not pick up JPA infrastructure:
 * {@code @WebMvcTest} loads only web {@code @Configuration}/{@code @Controller}
 * beans and skips this class.
 *
 * <ul>
 *   <li>{@code domain.model} — JPA entities</li>
 *   <li>{@code adapter.db} — Spring Data repository adapters (the port impls)</li>
 * </ul>
 */
@Configuration
@EntityScan(basePackages = "com.aitrack.server.domain.model")
@EnableJpaRepositories(basePackages = "com.aitrack.server.adapter.db")
public class JpaConfig {
}
