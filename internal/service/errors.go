package service

import "errors"

// 业务层统一错误定义
var (
	ErrDeviceNotFound   = errors.New("设备不存在")
	ErrDuplicateCommand = errors.New("指令重复下发（分布式锁）")
	ErrMQTTNotConnected = errors.New("MQTT未连接")
)
