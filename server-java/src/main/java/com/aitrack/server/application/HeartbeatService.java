package com.aitrack.server.application;

import com.aitrack.server.domain.model.HeartbeatRequest;
import com.aitrack.server.domain.model.DeviceEntity;
import com.aitrack.server.domain.model.TokenEntity;
import com.aitrack.server.domain.port.DevicePort;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;

@Service
@RequiredArgsConstructor
public class HeartbeatService {

    private final DevicePort deviceRepository;
    private final ObjectMapper objectMapper;

    @Transactional
    public void recordHeartbeat(TokenEntity token, HeartbeatRequest req) {
        DeviceEntity device = deviceRepository.findByDeviceId(req.getDeviceId())
            .orElseGet(() -> {
                DeviceEntity d = new DeviceEntity();
                d.setDeviceId(req.getDeviceId());
                d.setTokenKey(token.getTokenKey());
                d.setCreatedAt(Instant.now());
                return d;
            });

        device.setLastHeartbeat(Instant.now());
        device.setHostname(req.getHostname());
        device.setClientVersion(req.getClientVersion());
        if (req.getHooks() != null) {
            try {
                device.setHooksJson(objectMapper.writeValueAsString(req.getHooks()));
            } catch (JsonProcessingException e) {
                device.setHooksJson(null);
            }
        }
        deviceRepository.save(device);
    }
}
