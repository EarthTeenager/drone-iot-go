package model

import "time"

// Drone 无人机设备表 drones
// 职责4：MySQL数据表结构设计，设备ID、在线状态建立索引
type Drone struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceID  string    `gorm:"type:varchar(64);uniqueIndex:idx_device_id;not null;comment:设备唯一编号" json:"device_id"`
	Name      string    `gorm:"type:varchar(128);comment:设备名称/型号" json:"name"`
	Status    string    `gorm:"type:varchar(16);index:idx_status;default:'offline';comment:在线状态 online/offline" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (Drone) TableName() string {
	return "drones"
}
