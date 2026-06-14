package repository

import (
	"drone-iot-demo/internal/model"
)

// DroneRepo 无人机设备持久层
type DroneRepo struct{}

func NewDroneRepo() *DroneRepo {
	return &DroneRepo{}
}

// Create 注册无人机设备
func (r *DroneRepo) Create(d *model.Drone) error {
	return DB.Create(d).Error
}

// FindByDeviceID 通过设备ID查询，命中 idx_device_id 唯一索引
func (r *DroneRepo) FindByDeviceID(deviceID string) (*model.Drone, error) {
	var drone model.Drone
	err := DB.Where("device_id = ?", deviceID).First(&drone).Error
	if err != nil {
		return nil, err
	}
	return &drone, nil
}

// FindAll 查询全部设备列表
func (r *DroneRepo) FindAll() ([]model.Drone, error) {
	var drones []model.Drone
	err := DB.Find(&drones).Error
	return drones, err
}

// UpdateStatus 更新设备在线状态，命中 idx_status 索引
func (r *DroneRepo) UpdateStatus(deviceID string, status string) error {
	return DB.Model(&model.Drone{}).Where("device_id = ?", deviceID).Update("status", status).Error
}

// Delete 删除设备
func (r *DroneRepo) Delete(deviceID string) error {
	return DB.Where("device_id = ?", deviceID).Delete(&model.Drone{}).Error
}

// UpsertByDeviceID 不存在则插入、存在则更新状态（用于设备首次上报自动注册）
func (r *DroneRepo) UpsertByDeviceID(deviceID string, name string) error {
	drone := model.Drone{
		DeviceID: deviceID,
		Name:     name,
		Status:   "online",
	}
	return DB.Where("device_id = ?", deviceID).Assign(drone).FirstOrCreate(&drone).Error
}
