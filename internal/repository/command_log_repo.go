package repository

import (
	"drone-iot-demo/internal/model"
)

// CommandLogRepo 指令下发记录持久层
type CommandLogRepo struct{}

func NewCommandLogRepo() *CommandLogRepo {
	return &CommandLogRepo{}
}

// Insert 记录一条指令下发日志
func (r *CommandLogRepo) Insert(log *model.CommandLog) error {
	return DB.Create(log).Error
}

// UpdateStatus 更新指令执行状态，用于MQTT消息送达回调
func (r *CommandLogRepo) UpdateStatus(id uint, status string, msgID uint16) error {
	return DB.Model(&model.CommandLog{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"message_id": msgID,
		}).Error
}

// FindByDeviceID 按设备ID分页查询指令历史
func (r *CommandLogRepo) FindByDeviceID(deviceID string, limit, offset int) ([]model.CommandLog, int64, error) {
	var total int64
	var logs []model.CommandLog

	query := DB.Model(&model.CommandLog{}).Where("device_id = ?", deviceID)
	query.Count(&total)

	err := query.Order("create_time DESC").
		Limit(limit).Offset(offset).
		Find(&logs).Error
	return logs, total, err
}
