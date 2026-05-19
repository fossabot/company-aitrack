package com.aitrack.server.job;

import com.aitrack.server.entity.TokenEntity;
import com.aitrack.server.repository.TokenRepository;
import com.aitrack.server.service.ProfileService;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;

import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link ProfileAggregationJob}.
 * Uses Mockito's {@code @ExtendWith} so no Spring context is needed.
 */
@ExtendWith(MockitoExtension.class)
class ProfileAggregationJobTest {

    @Mock
    private ProfileService profileService;

    @Mock
    private TokenRepository tokenRepo;

    @InjectMocks
    private ProfileAggregationJob job;

    /**
     * Given 1 active + 1 inactive token, run() should call computeProfile exactly once
     * (only for the active token).
     */
    @Test
    void run_oneActiveOneInactive_computeProfileCalledOnce() {
        TokenEntity activeToken = new TokenEntity();
        activeToken.setTokenKey("active-tok");
        activeToken.setActive(true);

        TokenEntity inactiveToken = new TokenEntity();
        inactiveToken.setTokenKey("inactive-tok");
        inactiveToken.setActive(false);

        when(tokenRepo.findAll()).thenReturn(List.of(activeToken, inactiveToken));
        when(profileService.computeProfile(eq("active-tok"))).thenReturn(Optional.empty());

        job.run();

        verify(profileService, times(1)).computeProfile(eq("active-tok"));
        verify(profileService, never()).computeProfile(eq("inactive-tok"));
    }

    /**
     * Given 2 active tokens, run() should call computeProfile exactly twice.
     */
    @Test
    void run_twoActiveTokens_computeProfileCalledTwice() {
        TokenEntity t1 = new TokenEntity();
        t1.setTokenKey("tok-A");
        t1.setActive(true);

        TokenEntity t2 = new TokenEntity();
        t2.setTokenKey("tok-B");
        t2.setActive(true);

        when(tokenRepo.findAll()).thenReturn(List.of(t1, t2));
        when(profileService.computeProfile(any())).thenReturn(Optional.empty());

        job.run();

        verify(profileService, times(2)).computeProfile(any());
    }

    /**
     * Given no tokens, run() should not call computeProfile at all.
     */
    @Test
    void run_noTokens_computeProfileNotCalled() {
        when(tokenRepo.findAll()).thenReturn(List.of());

        job.run();

        verifyNoInteractions(profileService);
    }
}
