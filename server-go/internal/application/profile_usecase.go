package application

import (
	"encoding/json"
	"time"

	"github.com/aitrack/server/internal/domain/model"
	"github.com/aitrack/server/internal/domain/port"
)

// HeartbeatService records device heartbeat events.
type HeartbeatService struct {
	deviceRepo port.DevicePort
}

// NewHeartbeatService constructs the heartbeat use case.
func NewHeartbeatService(repo port.DevicePort) *HeartbeatService {
	return &HeartbeatService{deviceRepo: repo}
}

// RecordHeartbeat upserts the device state from a heartbeat request.
func (s *HeartbeatService) RecordHeartbeat(token *model.Token, req *model.HeartbeatRequest) error {
	existing, err := s.deviceRepo.FindByDeviceID(req.DeviceID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	d := &model.Device{
		DeviceID:      req.DeviceID,
		TokenKey:      token.TokenKey,
		Hostname:      req.Hostname,
		ClientVersion: req.ClientVersion,
		LastHeartbeat: &now,
		CreatedAt:     now,
	}
	if existing != nil {
		d.CreatedAt = existing.CreatedAt
	}
	if req.Hooks != nil {
		b, _ := json.Marshal(req.Hooks)
		d.HooksJSON = string(b)
	} else if existing != nil {
		d.HooksJSON = existing.HooksJSON
	}
	return s.deviceRepo.Upsert(d)
}

// StatsService aggregates edit stats and lists device state.
type StatsService struct {
	editRepo   port.EditRecordPort
	deviceRepo port.DevicePort
}

// NewStatsService constructs the stats use case.
func NewStatsService(editRepo port.EditRecordPort, deviceRepo port.DevicePort) *StatsService {
	return &StatsService{editRepo: editRepo, deviceRepo: deviceRepo}
}

// GetStats returns aggregated stats grouped by the given dimension.
func (s *StatsService) GetStats(groupBy string) ([]model.StatsRow, error) {
	var rows []port.StatsRow
	var err error
	switch groupBy {
	case "repo":
		rows, err = s.editRepo.AggregateByRepo()
	case "device":
		rows, err = s.editRepo.AggregateByDevice()
	case "hostname":
		rows, err = s.editRepo.AggregateByHostname()
	default:
		rows, err = s.editRepo.AggregateByTokenKey()
	}
	if err != nil {
		return nil, err
	}
	result := make([]model.StatsRow, len(rows))
	for i, r := range rows {
		result[i] = model.StatsRow{
			Group:        r.Group,
			Edits:        r.Edits,
			AddedLines:   r.AddedLines,
			RemovedLines: r.RemovedLines,
			Accepted:     r.Accepted,
			Flagged:      r.Flagged,
			Rejected:     r.Rejected,
			LastActive:   r.LastActive,
		}
	}
	return result, nil
}

// GetDevices lists all devices, marking those silent for more than 7 days.
func (s *StatsService) GetDevices() ([]model.DeviceInfo, error) {
	devices, err := s.deviceRepo.ListAll()
	if err != nil {
		return nil, err
	}
	silentThreshold := time.Now().Add(-7 * 24 * time.Hour)
	result := make([]model.DeviceInfo, len(devices))
	for i, d := range devices {
		silent := d.LastHeartbeat == nil || d.LastHeartbeat.Before(silentThreshold)
		result[i] = model.DeviceInfo{
			DeviceID:      d.DeviceID,
			TokenKey:      d.TokenKey,
			Hostname:      d.Hostname,
			ClientVersion: d.ClientVersion,
			LastHeartbeat: d.LastHeartbeat,
			HooksJSON:     d.HooksJSON,
			Silent:        silent,
		}
	}
	return result, nil
}
