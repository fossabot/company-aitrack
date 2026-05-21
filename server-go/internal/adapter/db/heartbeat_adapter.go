package dbadapter

import (
	"database/sql"
	"time"

	"github.com/aitrack/server/internal/domain/model"
	"github.com/aitrack/server/internal/domain/port"
)

// DeviceAdapter persists devices and heartbeats. It implements port.DevicePort.
type DeviceAdapter struct {
	db *sql.DB
}

// NewDeviceAdapter constructs a DeviceAdapter over the given database.
func NewDeviceAdapter(db *sql.DB) *DeviceAdapter {
	return &DeviceAdapter{db: db}
}

var _ port.DevicePort = (*DeviceAdapter)(nil)

// Upsert inserts or updates a device by its device ID.
func (r *DeviceAdapter) Upsert(d *model.Device) error {
	var lastHB interface{}
	if d.LastHeartbeat != nil {
		lastHB = d.LastHeartbeat.UTC().Format(time.RFC3339)
	}
	_, err := r.db.Exec(`
		INSERT INTO devices (device_id, token_key, hostname, client_version, last_heartbeat, hooks_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(device_id) DO UPDATE SET
		  token_key      = excluded.token_key,
		  hostname       = excluded.hostname,
		  client_version = excluded.client_version,
		  last_heartbeat = excluded.last_heartbeat,
		  hooks_json     = excluded.hooks_json`,
		d.DeviceID, d.TokenKey, d.Hostname, d.ClientVersion, lastHB, d.HooksJSON,
		d.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// FindByDeviceID returns a device by ID, or (nil, nil) when absent.
func (r *DeviceAdapter) FindByDeviceID(deviceID string) (*model.Device, error) {
	row := r.db.QueryRow(`
		SELECT id, device_id, token_key, hostname, client_version, last_heartbeat, hooks_json, created_at
		FROM devices WHERE device_id = $1`, deviceID)
	var d model.Device
	var lastHB, createdAt sql.NullString
	err := row.Scan(&d.ID, &d.DeviceID, &d.TokenKey, &d.Hostname, &d.ClientVersion, &lastHB, &d.HooksJSON, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastHB.Valid {
		t, _ := time.Parse(time.RFC3339, lastHB.String)
		d.LastHeartbeat = &t
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	return &d, nil
}

// ListAll returns every registered device.
func (r *DeviceAdapter) ListAll() ([]*model.Device, error) {
	rows, err := r.db.Query(`
		SELECT id, device_id, token_key, hostname, client_version, last_heartbeat, hooks_json, created_at
		FROM devices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*model.Device
	for rows.Next() {
		var d model.Device
		var lastHB, createdAt sql.NullString
		if err := rows.Scan(&d.ID, &d.DeviceID, &d.TokenKey, &d.Hostname, &d.ClientVersion, &lastHB, &d.HooksJSON, &createdAt); err != nil {
			return nil, err
		}
		if lastHB.Valid {
			t, _ := time.Parse(time.RFC3339, lastHB.String)
			d.LastHeartbeat = &t
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
		result = append(result, &d)
	}
	return result, rows.Err()
}
