package model

import "time"

// CommandLog 指令下发记录表 command_logs
// JD-岗位职责2：遥控指令下行精准下发，完整记录指令下发链路
type CommandLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceID   string    `gorm:"type:varchar(64);index:idx_cmd_device;not null;comment:目标设备编号" json:"device_id"`
	Command    string    `gorm:"type:varchar(64);not null;comment:指令类型 takeoff/land/return_home/lock_motor" json:"command"`
	Payload    string    `gorm:"type:text;comment:指令附加参数JSON" json:"payload"`
	Status     string    `gorm:"type:varchar(16);default:'pending';comment:执行状态 pending/sent/delivered/failed" json:"status"`
	MQTTQos    int       `gorm:"type:tinyint;default:2;comment:下发QoS等级" json:"mqtt_qos"`
	MessageID  uint16    `gorm:"comment:MQTT消息ID，用于追踪送达" json:"message_id"`
	CreateTime time.Time `gorm:"index:idx_cmd_time;autoCreateTime;comment:指令创建时间" json:"create_time"`
}

// TableName 指定表名
func (CommandLog) TableName() string {
	return "command_logs"
}
