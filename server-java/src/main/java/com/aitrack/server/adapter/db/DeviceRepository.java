package com.aitrack.server.adapter.db;

import com.aitrack.server.domain.model.DeviceEntity;
import com.aitrack.server.domain.port.DevicePort;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

/**
 * Spring Data JPA persistence adapter implementing {@link DevicePort}.
 */
@Repository
public interface DeviceRepository extends JpaRepository<DeviceEntity, Long>, DevicePort {

    // Most-specific re-declarations resolve the overload clash between
    // JpaRepository's generic CRUD signatures and the DevicePort port methods.
    @Override
    DeviceEntity save(DeviceEntity entity);

    @Override
    List<DeviceEntity> findAll();

    @Override
    Optional<DeviceEntity> findByDeviceId(String deviceId);

    @Override
    List<DeviceEntity> findByLastHeartbeatBefore(Instant threshold);
}
