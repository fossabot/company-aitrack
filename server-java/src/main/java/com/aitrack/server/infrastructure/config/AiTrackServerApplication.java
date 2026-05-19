package com.aitrack.server.infrastructure.config;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.scheduling.annotation.EnableScheduling;

/**
 * Application bootstrap and composition root.
 *
 * <p>This class lives in {@code infrastructure.config}; component scanning is
 * therefore declared explicitly so the whole {@code com.aitrack.server} hexagon
 * is wired: {@code domain}, {@code application}, {@code adapter} and
 * {@code infrastructure}.
 *
 * <p>JPA entity/repository scanning is intentionally delegated to the separate
 * {@link JpaConfig} class so web-layer slice tests stay free of JPA infrastructure.
 */
@SpringBootApplication(scanBasePackages = "com.aitrack.server")
@EnableConfigurationProperties
@EnableScheduling
public class AiTrackServerApplication {
    public static void main(String[] args) {
        SpringApplication.run(AiTrackServerApplication.class, args);
    }
}
