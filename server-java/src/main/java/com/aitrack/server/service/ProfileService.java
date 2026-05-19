package com.aitrack.server.service;

import com.aitrack.server.dto.ProfileDto;
import com.aitrack.server.dto.ProfileDto.DayCount;
import com.aitrack.server.dto.ProfileDto.DepthStats;
import com.aitrack.server.dto.ProfileDto.FrequencyStats;
import com.aitrack.server.entity.EditRecordEntity;
import com.aitrack.server.entity.EditRecordEntity.RecordStatus;
import com.aitrack.server.entity.TokenEntity;
import com.aitrack.server.repository.EditRecordRepository;
import com.aitrack.server.repository.TokenRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.time.LocalDate;
import java.time.ZoneOffset;
import java.util.*;
import java.util.stream.Collectors;

@Slf4j
@Service
@RequiredArgsConstructor
public class ProfileService {

    private final EditRecordRepository editRecordRepo;
    private final TokenRepository tokenRepo;

    /**
     * Compute a developer AI usage profile for the given tokenKey.
     *
     * @param tokenKey the 16-character token key (aitrack format)
     * @return Optional.empty() when neither records nor an active token exist; otherwise Optional.of(profile)
     */
    public Optional<ProfileDto> computeProfile(String tokenKey) {
        List<EditRecordEntity> records =
                editRecordRepo.findByTokenKeyAndStatusNot(tokenKey, RecordStatus.REJECTED);
        Optional<TokenEntity> tokenOpt = tokenRepo.findByTokenKeyAndActiveTrue(tokenKey);

        // 404: no records AND no active token
        if (records.isEmpty() && tokenOpt.isEmpty()) {
            return Optional.empty();
        }

        ProfileDto profile = new ProfileDto();
        profile.setTokenKey(tokenKey);
        profile.setGeneratedAt(Instant.now().toString());

        // Owner from token entity
        tokenOpt.ifPresent(t -> profile.setOwner(t.getOwner()));

        if (records.isEmpty()) {
            // Token exists but no records — return zero-value profile
            profile.setTotalEdits(0);
            profile.setTotalAddedLines(0);
            profile.setTotalRemovedLines(0);
            return Optional.of(profile);
        }

        // ------------------------------------------------------------------
        // Basic aggregates
        // ------------------------------------------------------------------
        profile.setTotalEdits(records.size());
        profile.setTotalAddedLines(records.stream().mapToLong(EditRecordEntity::getAddedLines).sum());
        profile.setTotalRemovedLines(records.stream().mapToLong(EditRecordEntity::getRemovedLines).sum());

        // ------------------------------------------------------------------
        // Time bounds (first_seen / last_seen)
        // ------------------------------------------------------------------
        List<Instant> instants = new ArrayList<>();
        for (EditRecordEntity e : records) {
            instants.add(parseTimestamp(e));
        }
        instants.sort(Comparator.naturalOrder());
        if (!instants.isEmpty()) {
            profile.setFirstSeen(instants.get(0).toString());
            profile.setLastSeen(instants.get(instants.size() - 1).toString());
        }

        // ------------------------------------------------------------------
        // Frequency stats
        // ------------------------------------------------------------------
        profile.setFrequency(computeFrequency(records));

        // ------------------------------------------------------------------
        // Depth stats
        // ------------------------------------------------------------------
        profile.setDepth(computeDepth(records));

        // ------------------------------------------------------------------
        // Scenario breakdown
        // ------------------------------------------------------------------
        Map<String, Long> scenarios = records.stream()
                .collect(Collectors.groupingBy(
                        e -> classifyScenario(e.getFilePath()),
                        Collectors.counting()
                ));
        profile.setScenarios(scenarios);

        // ------------------------------------------------------------------
        // Tools breakdown (skip null tools)
        // ------------------------------------------------------------------
        Map<String, Long> tools = records.stream()
                .filter(e -> e.getTool() != null)
                .collect(Collectors.groupingBy(
                        EditRecordEntity::getTool,
                        Collectors.counting()
                ));
        profile.setTools(tools.isEmpty() ? null : tools);

        return Optional.of(profile);
    }

    // -------------------------------------------------------------------------
    // Frequency computation
    // -------------------------------------------------------------------------

    private FrequencyStats computeFrequency(List<EditRecordEntity> records) {
        Instant now = Instant.now();
        Instant cutoff30d = now.minusSeconds(30L * 24 * 3600);
        Instant cutoff84d = now.minusSeconds(84L * 24 * 3600);

        long count30d = 0;
        long count84d = 0;
        Map<LocalDate, Long> dailyCounts = new TreeMap<>();

        for (EditRecordEntity e : records) {
            Instant ts = parseTimestamp(e);
            if (!ts.isBefore(cutoff30d)) {
                count30d++;
                LocalDate day = ts.atZone(ZoneOffset.UTC).toLocalDate();
                dailyCounts.merge(day, 1L, Long::sum);
            }
            if (!ts.isBefore(cutoff84d)) {
                count84d++;
            }
        }

        FrequencyStats freq = new FrequencyStats();
        freq.setDailyAvg30d(count30d / 30.0);
        freq.setWeeklyAvg12w(count84d / 12.0);

        List<DayCount> trend = dailyCounts.entrySet().stream()
                .map(entry -> new DayCount(entry.getKey().toString(), entry.getValue()))
                .collect(Collectors.toList());
        freq.setDailyTrend(trend);

        return freq;
    }

    // -------------------------------------------------------------------------
    // Depth computation
    // -------------------------------------------------------------------------

    private DepthStats computeDepth(List<EditRecordEntity> records) {
        List<Long> totals = records.stream()
                .map(e -> e.getAddedLines() + e.getRemovedLines())
                .sorted()
                .collect(Collectors.toList());

        DepthStats depth = new DepthStats();

        if (totals.isEmpty()) {
            return depth;
        }

        double avg = totals.stream().mapToLong(Long::longValue).average().orElse(0.0);
        depth.setAvgLines(avg);
        depth.setP50Lines(totals.get(totals.size() / 2));
        depth.setP90Lines(totals.get((int) (totals.size() * 0.9)));
        depth.setSmallCount(totals.stream().filter(t -> t < 10).count());
        depth.setMediumCount(totals.stream().filter(t -> t >= 10 && t <= 100).count());
        depth.setLargeCount(totals.stream().filter(t -> t > 100).count());

        return depth;
    }

    // -------------------------------------------------------------------------
    // Scenario classification
    // -------------------------------------------------------------------------

    /**
     * Classifies a file path into a scenario category.
     *
     * @param filePath the file path to classify (may be null/blank)
     * @return one of "test", "docs", "config", "feature", or "other"
     */
    String classifyScenario(String filePath) {
        if (filePath == null || filePath.isBlank()) {
            return "other";
        }
        String lowerPath = filePath.toLowerCase();

        if (lowerPath.contains("/test") || lowerPath.contains("_test.")
                || lowerPath.contains(".test.") || lowerPath.contains("/spec")
                || lowerPath.contains("_spec.") || lowerPath.contains(".spec.")) {
            return "test";
        }

        if (lowerPath.endsWith(".md") || lowerPath.endsWith(".rst") || lowerPath.endsWith(".txt")
                || lowerPath.contains("/docs/") || lowerPath.contains("/doc/")) {
            return "docs";
        }

        if (lowerPath.endsWith(".yaml") || lowerPath.endsWith(".yml") || lowerPath.endsWith(".toml")
                || lowerPath.endsWith(".json") || lowerPath.endsWith(".xml")
                || lowerPath.endsWith(".ini") || lowerPath.endsWith(".env")
                || lowerPath.endsWith(".properties") || lowerPath.contains("/config/")) {
            return "config";
        }

        return "feature";
    }

    // -------------------------------------------------------------------------
    // Timestamp parsing
    // -------------------------------------------------------------------------

    /**
     * Parses the entity's timestamp field (expected to be epoch-seconds as a Long string).
     * Falls back to receivedAt if parsing fails.
     */
    private Instant parseTimestamp(EditRecordEntity e) {
        try {
            long epochSec = Long.parseLong(e.getTimestamp());
            return Instant.ofEpochSecond(epochSec);
        } catch (NumberFormatException | NullPointerException ex) {
            log.debug("Failed to parse timestamp '{}' for record id={}, falling back to receivedAt",
                    e.getTimestamp(), e.getId());
            return e.getReceivedAt();
        }
    }
}
