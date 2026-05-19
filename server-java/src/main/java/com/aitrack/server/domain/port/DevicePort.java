package com.aitrack.server.domain.port;

import com.aitrack.server.domain.model.DeviceEntity;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

/**
 * Driven-side persistence port for client devices and heartbeats.
 *
 * <p>This is a pure domain interface (a secondary port of the hexagon).
 * The {@code adapter.db.DeviceRepository} Spring Data interface provides the
 * concrete implementation; domain and application code depend only on this port.
 */
public interface DevicePort {

    /** Persists a device; returns the saved instance. */
    DeviceEntity save(DeviceEntity device);

    /** Returns every registered device. */
    List<DeviceEntity> findAll();

    /** Returns a device by its device ID, if present. */
    Optional<DeviceEntity> findByDeviceId(String deviceId);

    /** Returns devices whose last heartbeat predates the given threshold. */
    List<DeviceEntity> findByLastHeartbeatBefore(Instant threshold);
}
