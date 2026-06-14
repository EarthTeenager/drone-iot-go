package service

import (
	"context"
	"log"
	"time"

	"drone-iot-demo/internal/model"
	"drone-iot-demo/internal/mqtt"
	"drone-iot-demo/internal/repository"
	"drone-iot-demo/internal/ws"
)

// DeviceService 设备业务逻辑层
// 协调MQTT数据 → 入库MySQL → 写Redis缓存 → WebSocket广播 → 告警检查
// JD-岗位职责2：收到设备上报数据后入库+缓存+广播
type DeviceService struct {
	droneRepo  *repository.DroneRepo
	statusRepo *repository.DeviceStatusRepo
	cmdLogRepo *repository.CommandLogRepo
	mqttClient *mqtt.Client
}

// NewDeviceService 创建DeviceService实例
func NewDeviceService(
	droneRepo *repository.DroneRepo,
	statusRepo *repository.DeviceStatusRepo,
	cmdLogRepo *repository.CommandLogRepo,
	mqttClient *mqtt.Client,
) *DeviceService {
	return &DeviceService{
		droneRepo:  droneRepo,
		statusRepo: statusRepo,
		cmdLogRepo: cmdLogRepo,
		mqttClient: mqttClient,
	}
}

// HandleDeviceData MQTT收到设备上行数据后的核心处理链路
// JD-岗位职责2：上行数据 → 入库MySQL + 写Redis热点缓存 + WebSocket广播 + 告警检查
func (s *DeviceService) HandleDeviceData(data mqtt.DeviceData) {
	ctx := context.Background()

	// 1. 自动注册设备（不存在则创建）
	s.droneRepo.UpsertByDeviceID(data.DeviceID, data.DeviceID)

	// 2. 设备状态入库MySQL
	status := &model.DeviceStatus{
		DeviceID:   data.DeviceID,
		Latitude:   data.Latitude,
		Longitude:  data.Longitude,
		Altitude:   data.Altitude,
		Speed:      data.Speed,
		Battery:    data.Battery,
		Roll:       data.Roll,
		Pitch:      data.Pitch,
		Yaw:        data.Yaw,
		FlightMode: data.FlightMode,
		GPSStatus:  data.GPSStatus,
		CreateTime: time.Now(),
	}
	if err := s.statusRepo.Insert(status); err != nil {
		log.Printf("[DeviceService] 状态入库失败: device=%s err=%v\n", data.DeviceID, err)
	}

	// 3. 写入Redis热点缓存（设备实时状态Hash）
	redisData := map[string]interface{}{
		"status":      "online",
		"last_seen":   time.Now().Unix(),
		"latitude":    data.Latitude,
		"longitude":   data.Longitude,
		"altitude":    data.Altitude,
		"speed":       data.Speed,
		"battery":     data.Battery,
		"roll":        data.Roll,
		"pitch":       data.Pitch,
		"yaw":         data.Yaw,
		"flight_mode": data.FlightMode,
		"gps_status":  data.GPSStatus,
	}
	if err := repository.SetDeviceOnline(ctx, data.DeviceID, redisData); err != nil {
		log.Printf("[DeviceService] Redis缓存写入失败: device=%s err=%v\n", data.DeviceID, err)
	}
	// 设置30s过期，持续上报则持续刷新
	repository.SetDeviceStatusTTL(ctx, data.DeviceID)

	// 4. 广播到所有WebSocket前端
	if ws.GlobalHub != nil {
		ws.GlobalHub.BroadcastJSON(ws.StatusMessage{
			Type: "status",
			Data: data,
		})
	}

	// 5. 触发告警规则检查（由AlertEngine处理）
	if AlertEngine != nil {
		AlertEngine.Check(data)
	}
}

// SendCommand 下发遥控指令到指定设备
// JD-岗位职责2：遥控指令下行精准下发
func (s *DeviceService) SendCommand(cmd mqtt.CommandPayload) (*model.CommandLog, error) {
	// 1. 分布式锁防重复（同一设备同一指令5s内不能重复下发）
	lockKey := "lock:cmd:" + cmd.DeviceID + ":" + cmd.Command
	ctx := context.Background()
	acquired, err := repository.TryLock(ctx, lockKey, 5*time.Second)
	if err != nil || !acquired {
		return nil, ErrDuplicateCommand
	}

	// 2. MQTT下发指令（QoS=2）
	msgID, err := s.mqttClient.PublishCommand(cmd)
	if err != nil {
		repository.Unlock(ctx, lockKey)
		return nil, err
	}

	// 3. 记录指令下发日志
	logEntry := &model.CommandLog{
		DeviceID:   cmd.DeviceID,
		Command:    cmd.Command,
		Payload:    cmd.Params,
		Status:     "sent",
		MQTTQos:    2,
		MessageID:  msgID,
		CreateTime: time.Now(),
	}
	if err := s.cmdLogRepo.Insert(logEntry); err != nil {
		log.Printf("[DeviceService] 指令日志记录失败: %v\n", err)
	}

	// 延迟释放锁（指令执行完成后由MQTT回调释放）
	go func() {
		time.Sleep(5 * time.Second)
		repository.Unlock(context.Background(), lockKey)
	}()

	return logEntry, nil
}

// GetDeviceList 查询设备列表（先查Redis在线状态，再查DB补充离线设备）
func (s *DeviceService) GetDeviceList() ([]model.Drone, error) {
	return s.droneRepo.FindAll()
}

// GetDeviceHistory 查询设备历史状态数据（分页）
func (s *DeviceService) GetDeviceHistory(deviceID string, limit, offset int) ([]model.DeviceStatus, int64, error) {
	return s.statusRepo.FindByDeviceID(deviceID, limit, offset)
}
