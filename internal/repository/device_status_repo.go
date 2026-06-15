package repository

import (
	"time"

	"drone-iot-demo/internal/model"
)

// DeviceStatusRepo 设备实时状态持久层
type DeviceStatusRepo struct{}

func NewDeviceStatusRepo() *DeviceStatusRepo {
	return &DeviceStatusRepo{}
}

// Insert 写入一条设备状态上报记录
func (r *DeviceStatusRepo) Insert(s *model.DeviceStatus) error {
	return DB.Create(s).Error
}

// FindByDeviceID 按设备ID分页查询历史状态，命中 idx_device_ts 联合索引
// 职责4：历史数据查询接口，利用索引避免全表扫描
func (r *DeviceStatusRepo) FindByDeviceID(deviceID string, limit, offset int) ([]model.DeviceStatus, int64, error) {
	var total int64
	var records []model.DeviceStatus

	query := DB.Model(&model.DeviceStatus{}).Where("device_id = ?", deviceID)
	query.Count(&total)

	err := query.Order("create_time DESC").
		Limit(limit).Offset(offset).
		Find(&records).Error
	return records, total, err
}

// FindByDeviceIDAndTimeRange 按设备和时间范围查询（用于告警回溯分析）
func (r *DeviceStatusRepo) FindByDeviceIDAndTimeRange(deviceID string, start, end time.Time) ([]model.DeviceStatus, error) {
	var records []model.DeviceStatus
	err := DB.Where("device_id = ? AND create_time BETWEEN ? AND ?", deviceID, start, end).
		Order("create_time ASC").
		Find(&records).Error
	return records, err
}

// FindLatestByDeviceID 查询指定设备最新一条状态记录
func (r *DeviceStatusRepo) FindLatestByDeviceID(deviceID string) (*model.DeviceStatus, error) {
	var status model.DeviceStatus
	err := DB.Where("device_id = ?", deviceID).
		Order("create_time DESC").First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}
