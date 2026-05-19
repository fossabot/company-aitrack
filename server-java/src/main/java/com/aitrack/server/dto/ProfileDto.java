package com.aitrack.server.dto;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Data;
import lombok.NoArgsConstructor;
import java.util.List;
import java.util.Map;

@Data
@NoArgsConstructor
public class ProfileDto {

    @JsonProperty("token_key")
    private String tokenKey;

    private String owner;

    @JsonProperty("total_edits")
    private long totalEdits;

    @JsonProperty("total_added_lines")
    private long totalAddedLines;

    @JsonProperty("total_removed_lines")
    private long totalRemovedLines;

    @JsonProperty("first_seen")
    private String firstSeen;          // ISO-8601 UTC string, null if no records

    @JsonProperty("last_seen")
    private String lastSeen;           // ISO-8601 UTC string

    @JsonProperty("generated_at")
    private String generatedAt;        // ISO-8601 UTC string

    private FrequencyStats frequency;
    private DepthStats depth;
    private Map<String, Long> languages;
    private Map<String, Long> tools;

    @JsonProperty("prompt_patterns")
    private Map<String, Long> promptPatterns;

    @Data
    @NoArgsConstructor
    public static class FrequencyStats {
        @JsonProperty("daily_avg_30d")
        private double dailyAvg30d;

        @JsonProperty("weekly_avg_12w")
        private double weeklyAvg12w;

        @JsonProperty("daily_trend")
        private List<DayCount> dailyTrend;
    }

    @Data
    public static class DayCount {
        private String date;   // "2026-05-19"
        private long count;

        public DayCount(String date, long count) {
            this.date = date;
            this.count = count;
        }
    }

    @Data
    @NoArgsConstructor
    public static class DepthStats {
        @JsonProperty("avg_lines")
        private double avgLines;

        @JsonProperty("p50_lines")
        private long p50Lines;

        @JsonProperty("p90_lines")
        private long p90Lines;

        @JsonProperty("small_count")
        private long smallCount;    // total < 10 lines

        @JsonProperty("medium_count")
        private long mediumCount;   // 10 <= total <= 100

        @JsonProperty("large_count")
        private long largeCount;    // total > 100

        @JsonProperty("comment_density")
        private double commentDensity;  // ratio: comment lines added / total lines added, 0.0–1.0
    }
}
