package model

import "time"

// DeviceStatus 设备实时状态上报表 device_status
// JD-岗位职责4：数据表结构设计，按时间建立索引用于历史数据查询
type DeviceStatus struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceID   string    `gorm:"type:varchar(64);index:idx_device_ts,priority:1;not null;comment:设备编号" json:"device_id"`
	Latitude   float64   `gorm:"type:decimal(10,6);comment:纬度" json:"latitude"`
	Longitude  float64   `gorm:"type:decimal(10,6);comment:经度" json:"longitude"`
	Altitude   float64   `gorm:"type:decimal(10,2);comment:高度(m)" json:"altitude"`
	Speed      float64   `gorm:"type:decimal(8,2);comment:速度(m/s)" json:"speed"`
	Battery    int       `gorm:"type:int;comment:电量百分比 0-100" json:"battery"`
	Roll       float64   `gorm:"type:decimal(6,2);comment:横滚角" json:"roll"`
	Pitch      float64   `gorm:"type:decimal(6,2);comment:俯仰角" json:"pitch"`
	Yaw        float64   `gorm:"type:decimal(6,2);comment:偏航角" json:"yaw"`
	FlightMode string    `gorm:"type:varchar(32);comment:飞行模式" json:"flight_mode"`
	GPSStatus  int       `gorm:"type:tinyint;default:0;comment:GPS信号 0-5" json:"gps_status"`
	CreateTime time.Time `gorm:"index:idx_device_ts,priority:2;autoCreateTime;comment:上报时间" json:"create_time"`
}

// TableName 指定表名
func (DeviceStatus) TableName() string {
	return "device_status"
}
