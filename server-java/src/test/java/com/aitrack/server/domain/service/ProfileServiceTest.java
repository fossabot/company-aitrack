package com.aitrack.server.domain.service;

import com.aitrack.server.domain.model.ProfileDto;
import com.aitrack.server.domain.model.EditRecordEntity;
import com.aitrack.server.domain.model.EditRecordEntity.RecordStatus;
import com.aitrack.server.domain.model.TokenEntity;
import com.aitrack.server.domain.port.EditRecordPort;
import com.aitrack.server.domain.port.TokenPort;
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
    private EditRecordPort editRecordRepo;

    @Mock
    private TokenPort tokenRepo;

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
    // Test 5: detectLanguage — Python
    // -------------------------------------------------------------------------

    @Test
    void detectLanguage_pythonFile_returnsPython() {
        assertThat(profileService.detectLanguage("src/main.py")).isEqualTo("Python");
    }

    // -------------------------------------------------------------------------
    // Test 6: detectLanguage — TypeScript
    // -------------------------------------------------------------------------

    @Test
    void detectLanguage_typescriptFile_returnsTypeScript() {
        assertThat(profileService.detectLanguage("app/index.ts")).isEqualTo("TypeScript");
        assertThat(profileService.detectLanguage("components/Button.tsx")).isEqualTo("TypeScript");
    }

    // -------------------------------------------------------------------------
    // Test 7: detectLanguage — Java
    // -------------------------------------------------------------------------

    @Test
    void detectLanguage_javaFile_returnsJava() {
        assertThat(profileService.detectLanguage("src/main/java/com/example/Service.java")).isEqualTo("Java");
    }

    // -------------------------------------------------------------------------
    // Test 8: detectLanguage — Go
    // -------------------------------------------------------------------------

    @Test
    void detectLanguage_goFile_returnsGo() {
        assertThat(profileService.detectLanguage("cmd/server/main.go")).isEqualTo("Go");
    }

    // -------------------------------------------------------------------------
    // Test 9: detectLanguage — Rust
    // -------------------------------------------------------------------------

    @Test
    void detectLanguage_rustFile_returnsRust() {
        assertThat(profileService.detectLanguage("src/lib.rs")).isEqualTo("Rust");
    }

    // -------------------------------------------------------------------------
    // Test 9b: detectLanguage — unknown extension → "Other"
    // -------------------------------------------------------------------------

    @Test
    void detectLanguage_unknownExtension_returnsOther() {
        assertThat(profileService.detectLanguage("file.unknown")).isEqualTo("Other");
    }

    // -------------------------------------------------------------------------
    // Test 9c: detectLanguage — null/blank → "Other"
    // -------------------------------------------------------------------------

    @Test
    void detectLanguage_nullOrBlank_returnsOther() {
        assertThat(profileService.detectLanguage(null)).isEqualTo("Other");
        assertThat(profileService.detectLanguage("")).isEqualTo("Other");
        assertThat(profileService.detectLanguage("   ")).isEqualTo("Other");
    }

    // -------------------------------------------------------------------------
    // Test 9d: computeCommentDensity — diff_hunk with no "+" lines → 0.0
    // -------------------------------------------------------------------------

    @Test
    void computeCommentDensity_noAddedLines_returnsZero() {
        EditRecordEntity r = makeRecord("tokD1", "claude", "src/A.java", 0, 2, epochNowMinus(1));
        r.setDiffHunk("-old line\n-another old line");

        double density = profileService.computeCommentDensity(List.of(r));

        assertThat(density).isEqualTo(0.0);
    }

    // -------------------------------------------------------------------------
    // Test 9e: computeCommentDensity — 2 comment lines + 3 code lines → 0.4
    // -------------------------------------------------------------------------

    @Test
    void computeCommentDensity_twoCommentThreeCode_returnsPointFour() {
        EditRecordEntity r = makeRecord("tokD2", "claude", "src/B.java", 5, 0, epochNowMinus(1));
        r.setDiffHunk("+// first comment\n+// second comment\n+int x = 1;\n+int y = 2;\n+return x + y;");

        double density = profileService.computeCommentDensity(List.of(r));

        assertThat(density).isEqualTo(0.4);
    }

    // -------------------------------------------------------------------------
    // Test 9f: computeCommentDensity — null diff_hunk → 0.0
    // -------------------------------------------------------------------------

    @Test
    void computeCommentDensity_nullDiffHunk_returnsZero() {
        EditRecordEntity r = makeRecord("tokD3", "claude", "src/C.py", 3, 0, epochNowMinus(1));
        r.setDiffHunk(null);

        double density = profileService.computeCommentDensity(List.of(r));

        assertThat(density).isEqualTo(0.0);
    }

    // -------------------------------------------------------------------------
    // Test 9g: computeCommentDensity — "#" comments mixed with code → correct ratio
    // -------------------------------------------------------------------------

    @Test
    void computeCommentDensity_hashCommentsMixed_correctRatio() {
        EditRecordEntity r = makeRecord("tokD4", "claude", "src/D.py", 4, 0, epochNowMinus(1));
        // 1 comment (#), 3 code lines → 0.25
        r.setDiffHunk("+# this is a comment\n+x = 1\n+y = 2\n+z = x + y");

        double density = profileService.computeCommentDensity(List.of(r));

        assertThat(density).isEqualTo(0.25);
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
    // Test 13: computePromptPatterns — "implement" keyword → generate count=1
    // -------------------------------------------------------------------------

    @Test
    void computePromptPatterns_generate_matches() {
        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tokP1"), eq(RecordStatus.REJECTED)))
                .thenReturn(List.of(makeRecordWithPrompt("tokP1", "implement new auth feature")));
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tokP1")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tokP1");

        assertThat(result).isPresent();
        java.util.Map<String, Long> patterns = result.get().getPromptPatterns();
        assertThat(patterns).isNotNull();
        assertThat(patterns.get("generate")).isEqualTo(1L);
        assertThat(patterns.get("fix_debug")).isEqualTo(0L);
    }

    // -------------------------------------------------------------------------
    // Test 14: computePromptPatterns — "fix" + "bug" keywords → fix_debug count=1
    // -------------------------------------------------------------------------

    @Test
    void computePromptPatterns_fix_debug_matches() {
        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tokP2"), eq(RecordStatus.REJECTED)))
                .thenReturn(List.of(makeRecordWithPrompt("tokP2", "fix the login bug")));
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tokP2")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tokP2");

        assertThat(result).isPresent();
        java.util.Map<String, Long> patterns = result.get().getPromptPatterns();
        assertThat(patterns).isNotNull();
        assertThat(patterns.get("fix_debug")).isEqualTo(1L);
        assertThat(patterns.get("generate")).isEqualTo(0L);
    }

    // -------------------------------------------------------------------------
    // Test 15: computePromptPatterns — null prompt_summary → all counts=0
    // -------------------------------------------------------------------------

    @Test
    void computePromptPatterns_null_prompt_skipped() {
        when(editRecordRepo.findByTokenKeyAndStatusNot(eq("tokP3"), eq(RecordStatus.REJECTED)))
                .thenReturn(List.of(makeRecordWithPrompt("tokP3", null)));
        when(tokenRepo.findByTokenKeyAndActiveTrue(eq("tokP3")))
                .thenReturn(Optional.empty());

        Optional<ProfileDto> result = profileService.computeProfile("tokP3");

        assertThat(result).isPresent();
        java.util.Map<String, Long> patterns = result.get().getPromptPatterns();
        assertThat(patterns).isNotNull();
        assertThat(patterns.values()).allMatch(v -> v == 0L);
    }

    // -------------------------------------------------------------------------
    // Test 16: classifyPrompt — Chinese generate keyword
    // -------------------------------------------------------------------------

    @Test
    void classifyPrompt_chineseGenerate_returnsGenerate() {
        assertThat(profileService.classifyPrompt("帮我生成一个 REST API")).isEqualTo("generate");
    }

    // -------------------------------------------------------------------------
    // Test 17: classifyPrompt — Chinese fix_debug keyword
    // -------------------------------------------------------------------------

    @Test
    void classifyPrompt_chineseFixDebug_returnsFixDebug() {
        assertThat(profileService.classifyPrompt("修复这个错误")).isEqualTo("fix_debug");
    }

    // -------------------------------------------------------------------------
    // Test 18: classifyPrompt — Chinese test keyword
    // -------------------------------------------------------------------------

    @Test
    void classifyPrompt_chineseTest_returnsTest() {
        assertThat(profileService.classifyPrompt("写单元测试")).isEqualTo("test");
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

    private EditRecordEntity makeRecordWithPrompt(String tokenKey, String promptSummary) {
        EditRecordEntity e = makeRecord(tokenKey, "claude", "src/Foo.java", 5, 2, epochNowMinus(1));
        e.setPromptSummary(promptSummary);
        return e;
    }
}
