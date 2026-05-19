package com.aitrack.server.application;

import com.aitrack.server.domain.port.TokenPort;
import com.aitrack.server.domain.service.ProfileService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class ProfileAggregationJob {

    private final ProfileService profileService;
    private final TokenPort tokenRepo;

    /**
     * Daily profile aggregation — runs at 02:00 server time.
     * Phase 3: on-demand computation (warm-up). Phase 4 will persist results.
     */
    @Scheduled(cron = "0 0 2 * * *")
    public void run() {
        log.info("[profile-job] daily profile aggregation started");
        long count = tokenRepo.findAll().stream()
            .filter(t -> t.isActive())
            .peek(t -> profileService.computeProfile(t.getTokenKey()))
            .count();
        log.info("[profile-job] aggregated {} active token profiles", count);
    }
}
