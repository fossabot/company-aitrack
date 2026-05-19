package com.aitrack.server.adapter.handler;

import com.aitrack.server.application.StatsService;
import com.aitrack.server.domain.model.DeviceInfo;
import com.aitrack.server.domain.model.StatsRow;
import jakarta.servlet.http.HttpServletRequest;
import lombok.RequiredArgsConstructor;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/api/v1/ai-track")
@RequiredArgsConstructor
public class StatsController {

    private final RequestAuthHelper authHelper;
    private final StatsService statsService;

    @GetMapping("/stats")
    public ResponseEntity<List<StatsRow>> stats(
        HttpServletRequest httpRequest,
        @RequestParam(defaultValue = "token") String group_by
    ) {
        authHelper.resolveToken(httpRequest);
        return ResponseEntity.ok(statsService.getStats(group_by));
    }

    @GetMapping("/devices")
    public ResponseEntity<List<DeviceInfo>> devices(HttpServletRequest httpRequest) {
        authHelper.resolveToken(httpRequest);
        return ResponseEntity.ok(statsService.getDevices());
    }
}
