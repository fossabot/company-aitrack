package com.aitrack.server.service;

import com.aitrack.server.dto.ProfileDto;
import com.aitrack.server.entity.EditRecordEntity;
import com.aitrack.server.entity.EditRecordEntity.RecordStatus;
import com.aitrack.server.entity.TokenEntity;
import com.aitrack.server.repository.EditRecordRepository;
import com.aitrack.server.repository.TokenRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;

/**
 * Unit tests for {@link ProfileService}.
 * Uses Mockito's {@code @ExtendWith} so no Spring context is needed.
 */
@ExtendWith(MockitoExtension.class)
class ProfileServiceTest {

    @Mock
    private EditRecordRepository editRecordRepo;

    @Mock
    private TokenRepository tokenRepo;

    @InjectMocks
    private ProfileService profileService;

    // -------------------------------------------------------------------------
    // Test 1: No records + no token → Optional.empty()
    // -------------------------------------------------------------------------

    @Test
    void computeProfile_noRecordsNoToken_returnsEmpty() {
        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tok1"), eq(RecordStatus.REJECTED)))
                .thenReturn(List.of());
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tok1")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tok1");

        assertThat(result).isEmpty();
    }

    // -------------------------------------------------------------------------
    // Test 2: Token exists + empty records → Optional.of(profile) with zero totals
    // -------------------------------------------------------------------------

    @Test
    void computeProfile_tokenExistsNoRecords_returnsZeroProfile() {
        TokenEntity token = new TokenEntity();
        token.setTokenKey("tok2");
        token.setOwner("bob");
        token.setActive(true);

        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tok2"), eq(RecordStatus.REJECTED)))
                .thenReturn(List.of());
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tok2")))
                .thenReturn(Optional.of(token));

        Optional<ProfileDto> result = profileService.computeProfile("tok2");

        assertThat(result).isPresent();
        ProfileDto profile = result.get();
        assertThat(profile.getTotalEdits()).isZero();
        assertThat(profile.getTotalAddedLines()).isZero();
        assertThat(profile.getTotalRemovedLines()).isZero();
        assertThat(profile.getOwner()).isEqualTo("bob");
        assertThat(profile.getFrequency()).isNull();
        assertThat(profile.getDepth()).isNull();
    }

    // -------------------------------------------------------------------------
    // Test 3: 3 records with different tools → tools map has correct counts
    // -------------------------------------------------------------------------

    @Test
    void computeProfile_threeRecordsDifferentTools_toolCountsCorrect() {
        EditRecordEntity r1 = makeRecord("tok3", "claude", "src/Foo.java", 5, 2, epochNowMinus(1));
        EditRecordEntity r2 = makeRecord("tok3", "claude", "src/Bar.java", 3, 1, epochNowMinus(2));
        EditRecordEntity r3 = makeRecord("tok3", "cursor", "src/Baz.java", 7, 0, epochNowMinus(3));

        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tok3"), eq(RecordStatus.REJECTED)))
                .thenReturn(List.of(r1, r2, r3));
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tok3")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tok3");

        assertThat(result).isPresent();
        ProfileDto profile = result.get();
        assertThat(profile.getTools()).containsEntry("claude", 2L);
        assertThat(profile.getTools()).containsEntry("cursor", 1L);
        assertThat(profile.getTotalEdits()).isEqualTo(3);
    }

    // -------------------------------------------------------------------------
    // Test 4: Records spanning last 30 days → daily_avg_30d is correct
    // -------------------------------------------------------------------------

    @Test
    void computeProfile_recordsInLast30Days_dailyAvgCorrect() {
        // 6 records within the last 30 days
        List<EditRecordEntity> records = List.of(
                makeRecord("tok4", "claude", "src/A.java", 1, 0, epochNowMinus(1)),
                makeRecord("tok4", "claude", "src/B.java", 1, 0, epochNowMinus(5)),
                makeRecord("tok4", "claude", "src/C.java", 1, 0, epochNowMinus(10)),
                makeRecord("tok4", "claude", "src/D.java", 1, 0, epochNowMinus(15)),
                makeRecord("tok4", "claude", "src/E.java", 1, 0, epochNowMinus(20)),
                makeRecord("tok4", "claude", "src/F.java", 1, 0, epochNowMinus(25))
        );

        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tok4"), eq(RecordStatus.REJECTED)))
                .thenReturn(records);
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tok4")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tok4");

        assertThat(result).isPresent();
        ProfileDto profile = result.get();
        assertThat(profile.getFrequency()).isNotNull();
        // 6 records / 30 days = 0.2
        assertThat(profile.getFrequency().getDailyAvg30d()).isEqualTo(6.0 / 30.0);
    }

    // -------------------------------------------------------------------------
    // Test 5: classifyScenario — test path → "test"
    // -------------------------------------------------------------------------

    @Test
    void classifyScenario_testPath_returnsTest() {
        assertThat(profileService.classifyScenario("src/test/java/MyTest.java")).isEqualTo("test");
        assertThat(profileService.classifyScenario("src/MyService_test.go")).isEqualTo("test");
        assertThat(profileService.classifyScenario("lib/foo.test.ts")).isEqualTo("test");
        assertThat(profileService.classifyScenario("src/spec/my_spec.rb")).isEqualTo("test");
    }

    // -------------------------------------------------------------------------
    // Test 6: classifyScenario — ".md" file → "docs"
    // -------------------------------------------------------------------------

    @Test
    void classifyScenario_mdFile_returnsDocs() {
        assertThat(profileService.classifyScenario("README.md")).isEqualTo("docs");
        assertThat(profileService.classifyScenario("docs/guide.rst")).isEqualTo("docs");
        assertThat(profileService.classifyScenario("CHANGELOG.txt")).isEqualTo("docs");
        assertThat(profileService.classifyScenario("src/docs/api.md")).isEqualTo("docs");
    }

    // -------------------------------------------------------------------------
    // Test 7: classifyScenario — ".yaml" file → "config"
    // -------------------------------------------------------------------------

    @Test
    void classifyScenario_yamlFile_returnsConfig() {
        assertThat(profileService.classifyScenario("application.yaml")).isEqualTo("config");
        assertThat(profileService.classifyScenario("docker-compose.yml")).isEqualTo("config");
        assertThat(profileService.classifyScenario("config/settings.toml")).isEqualTo("config");
        assertThat(profileService.classifyScenario("pom.xml")).isEqualTo("config");
        assertThat(profileService.classifyScenario("package.json")).isEqualTo("config");
        assertThat(profileService.classifyScenario(".env")).isEqualTo("config");
        assertThat(profileService.classifyScenario("src/config/main.java")).isEqualTo("config");
    }

    // -------------------------------------------------------------------------
    // Test 8: classifyScenario — Java source file → "feature"
    // -------------------------------------------------------------------------

    @Test
    void classifyScenario_javaSourceFile_returnsFeature() {
        assertThat(profileService.classifyScenario("src/main/java/com/example/Service.java")).isEqualTo("feature");
        assertThat(profileService.classifyScenario("src/main/kotlin/com/example/Controller.kt")).isEqualTo("feature");
    }

    // -------------------------------------------------------------------------
    // Test 9: classifyScenario — null → "other"
    // -------------------------------------------------------------------------

    @Test
    void classifyScenario_null_returnsOther() {
        assertThat(profileService.classifyScenario(null)).isEqualTo("other");
        assertThat(profileService.classifyScenario("")).isEqualTo("other");
        assertThat(profileService.classifyScenario("   ")).isEqualTo("other");
    }

    // -------------------------------------------------------------------------
    // Test 10: Depth — sorted list p50/p90 correct
    // -------------------------------------------------------------------------

    @Test
    void computeProfile_depthPercentiles_correct() {
        // 10 records with total lines: 1,2,3,4,5,6,7,8,9,10 (sorted)
        List<EditRecordEntity> records = List.of(
                makeRecord("tok10", "claude", "src/A.java", 1, 0, epochNowMinus(1)),
                makeRecord("tok10", "claude", "src/B.java", 2, 0, epochNowMinus(2)),
                makeRecord("tok10", "claude", "src/C.java", 3, 0, epochNowMinus(3)),
                makeRecord("tok10", "claude", "src/D.java", 4, 0, epochNowMinus(4)),
                makeRecord("tok10", "claude", "src/E.java", 5, 0, epochNowMinus(5)),
                makeRecord("tok10", "claude", "src/F.java", 6, 0, epochNowMinus(6)),
                makeRecord("tok10", "claude", "src/G.java", 7, 0, epochNowMinus(7)),
                makeRecord("tok10", "claude", "src/H.java", 8, 0, epochNowMinus(8)),
                makeRecord("tok10", "claude", "src/I.java", 9, 0, epochNowMinus(9)),
                makeRecord("tok10", "claude", "src/J.java", 10, 0, epochNowMinus(10))
        );

        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tok10"), eq(RecordStatus.REJECTED)))
                .thenReturn(records);
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tok10")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tok10");

        assertThat(result).isPresent();
        ProfileDto.DepthStats depth = result.get().getDepth();
        assertThat(depth).isNotNull();
        // p50: index 10/2=5 → value at index 5 in sorted [1,2,3,4,5,6,7,8,9,10] = 6
        assertThat(depth.getP50Lines()).isEqualTo(6);
        // p90: index (int)(10*0.9)=9 → value at index 9 = 10
        assertThat(depth.getP90Lines()).isEqualTo(10);
        // avg: (1+2+...+10)/10 = 5.5
        assertThat(depth.getAvgLines()).isEqualTo(5.5);
    }

    // -------------------------------------------------------------------------
    // Test 11: Depth — small/medium/large buckets correct
    // -------------------------------------------------------------------------

    @Test
    void computeProfile_depthBuckets_correct() {
        List<EditRecordEntity> records = List.of(
                makeRecord("tok11", "claude", "src/A.java", 5, 3, epochNowMinus(1)),   // total=8  → small
                makeRecord("tok11", "claude", "src/B.java", 1, 1, epochNowMinus(2)),   // total=2  → small
                makeRecord("tok11", "claude", "src/C.java", 50, 20, epochNowMinus(3)), // total=70 → medium
                makeRecord("tok11", "claude", "src/D.java", 10, 0, epochNowMinus(4)),  // total=10 → medium
                makeRecord("tok11", "claude", "src/E.java", 100, 0, epochNowMinus(5)), // total=100 → medium
                makeRecord("tok11", "claude", "src/F.java", 200, 5, epochNowMinus(6))  // total=205 → large
        );

        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tok11"), eq(RecordStatus.REJECTED)))
                .thenReturn(records);
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tok11")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tok11");

        assertThat(result).isPresent();
        ProfileDto.DepthStats depth = result.get().getDepth();
        assertThat(depth.getSmallCount()).isEqualTo(2);   // <10: 8, 2
        assertThat(depth.getMediumCount()).isEqualTo(3);  // 10<=x<=100: 70, 10, 100
        assertThat(depth.getLargeCount()).isEqualTo(1);   // >100: 205
    }

    // -------------------------------------------------------------------------
    // Test 12: Records with invalid timestamp → fallback to receivedAt
    // -------------------------------------------------------------------------

    @Test
    void computeProfile_invalidTimestamp_fallsBackToReceivedAt() {
        EditRecordEntity record = new EditRecordEntity();
        record.setId(1L);
        record.setTokenKey("tok12");
        record.setTool("claude");
        record.setFilePath("src/Foo.java");
        record.setAddedLines(5);
        record.setRemovedLines(2);
        record.setTimestamp("not-a-number");  // invalid timestamp
        record.setReceivedAt(Instant.now().minusSeconds(100));
        record.setStatus(RecordStatus.ACCEPTED);

        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tok12"), eq(RecordStatus.REJECTED)))
                .thenReturn(List.of(record));
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tok12")))
                .thenReturn(Optional.empty());

        // Should not throw — fallback to receivedAt
        Optional<ProfileDto> result = profileService.computeProfile("tok12");

        assertThat(result).isPresent();
        assertThat(result.get().getTotalEdits()).isEqualTo(1);
        assertThat(result.get().getFirstSeen()).isNotNull();
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    private EditRecordEntity makeRecord(String tokenKey, String tool, String filePath,
                                        long addedLines, long removedLines, long epochSec) {
        EditRecordEntity e = new EditRecordEntity();
        e.setTokenKey(tokenKey);
        e.setTool(tool);
        e.setFilePath(filePath);
        e.setAddedLines(addedLines);
        e.setRemovedLines(removedLines);
        e.setTimestamp(String.valueOf(epochSec));
        e.setReceivedAt(Instant.ofEpochSecond(epochSec));
        e.setStatus(RecordStatus.ACCEPTED);
        return e;
    }

    private long epochNowMinus(int days) {
        return Instant.now().minus(days, ChronoUnit.DAYS).getEpochSecond();
    }
}
