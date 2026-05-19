package port

import "github.com/aitrack/server/internal/domain/model"

// DevicePort is the persistence port for client devices and heartbeats.
type DevicePort interface {
	// Upsert inserts or updates a device by its device ID.
	Upsert(d *model.Device) error
	// FindByDeviceID returns a device by ID, or (nil, nil) when absent.
	FindByDeviceID(deviceID string) (*model.Device, error)
	// ListAll returns every registered device.
	ListAll() ([]*model.Device, error)
}
