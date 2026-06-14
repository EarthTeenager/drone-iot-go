package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"drone-iot-demo/internal/mqtt"
	"drone-iot-demo/internal/repository"
	"drone-iot-demo/internal/ws"
)

// AlertEngine 全局告警引擎单例
var AlertEngine *AlertService

// ==================== 告警规则引擎 ====================
// JD-岗位职责3：设备状态、告警数据服务端主动向前端实时推送
// JD-加分项3：架构预留扩展点，AlertRule接口化设计

// AlertRule 告警规则接口（可扩展，方便后续新增自定义告警规则）
type AlertRule interface {
	Name() string                              // 规则名称
	Check(data mqtt.DeviceData) (bool, string) // 检查数据，返回是否触发告警+告警消息
}

// AlertService 告警引擎，管理所有告警规则
type AlertService struct {
	mu        sync.RWMutex
	rules     []AlertRule
	lastAlert map[string]time.Time // 设备告警冷却期，避免重复告警
}

// NewAlertService 创建告警引擎
func NewAlertService() *AlertService {
	as := &AlertService{
		rules:     make([]AlertRule, 0),
		lastAlert: make(map[string]time.Time),
	}
	// 注册默认3条告警规则
	as.RegisterRule(&LowBatteryRule{})
	as.RegisterRule(&AltitudeChangeRule{})
	as.RegisterRule(&GPSDriftRule{})
	AlertEngine = as
	return as
}

// RegisterRule 注册告警规则
func (as *AlertService) RegisterRule(rule AlertRule) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.rules = append(as.rules, rule)
	log.Printf("[Alert] 规则已注册: %s\n", rule.Name())
}

// Check 对所有规则逐一检查数据，触发告警则广播+记录
// 由service层在收到MQTT数据后调用
func (as *AlertService) Check(data mqtt.DeviceData) {
	as.mu.RLock()
	rules := as.rules
	as.mu.RUnlock()

	for _, rule := range rules {
		hit, msg := rule.Check(data)
		if hit {
			// 告警冷却期：同一设备同一规则30s内不重复告警
			coolKey := data.DeviceID + ":" + rule.Name()
			as.mu.Lock()
			lastTime, exists := as.lastAlert[coolKey]
			if exists && time.Since(lastTime) < 30*time.Second {
				as.mu.Unlock()
				continue
			}
			as.lastAlert[coolKey] = time.Now()
			as.mu.Unlock()

			// 构造告警消息
			alertMsg := map[string]interface{}{
				"type":      "alert",
				"device_id": data.DeviceID,
				"rule":      rule.Name(),
				"message":   msg,
				"timestamp": time.Now().Unix(),
			}
			log.Printf("[Alert] 告警触发: device=%s rule=%s msg=%s\n",
				data.DeviceID, rule.Name(), msg)

			// 1. WebSocket广播给前端
			if ws.GlobalHub != nil {
				ws.GlobalHub.BroadcastJSON(alertMsg)
			}

			// 2. Redis记录告警历史
			alertJSON := fmt.Sprintf(
				`{"device_id":"%s","rule":"%s","message":"%s","timestamp":%d}`,
				data.DeviceID, rule.Name(), msg, time.Now().Unix(),
			)
			repository.PushAlert(context.Background(), alertJSON)
		}
	}
}

// ==================== 内置告警规则实现 ====================

// LowBatteryRule 低电量告警：电量低于20%触发
type LowBatteryRule struct{}

func (r *LowBatteryRule) Name() string {
	return "低电量告警"
}
func (r *LowBatteryRule) Check(data mqtt.DeviceData) (bool, string) {
	if data.Battery < 20 {
		return true, fmt.Sprintf("无人机 %s 电量过低: %d%%", data.DeviceID, data.Battery)
	}
	return false, ""
}

// AltitudeChangeRule 高度突变告警：单次上报高度变化超过50m
type AltitudeChangeRule struct {
	rwMu         sync.RWMutex
	lastAltitude map[string]float64 // 记录上次高度
}

func (r *AltitudeChangeRule) Name() string {
	return "高度突变告警"
}
func (r *AltitudeChangeRule) Check(data mqtt.DeviceData) (bool, string) {
	if r.lastAltitude == nil {
		r.lastAltitude = make(map[string]float64)
	}
	r.rwMu.RLock()
	prev, exists := r.lastAltitude[data.DeviceID]
	r.rwMu.RUnlock()
	r.rwMu.Lock()
	defer r.rwMu.Unlock()
	if r.lastAltitude == nil {
		r.lastAltitude = make(map[string]float64)
	}
	r.lastAltitude[data.DeviceID] = data.Altitude
	if exists && abs(data.Altitude-prev) > 50 {
		return true, fmt.Sprintf("无人机 %s 高度突变: %.1fm → %.1fm", data.DeviceID, prev, data.Altitude)
	}
	return false, ""
}

// GPSDriftRule GPS漂移告警：坐标偏移超过100m
type GPSDriftRule struct {
	rw      sync.RWMutex
	lastLat map[string]float64
	lastLng map[string]float64
}

// 分片数量，可根据QPS调整，32/64足够绝大多数IoT场景
const gpsShardNum = 32

func (r *GPSDriftRule) Name() string {
	return "GPS漂移告警"
}
func (r *GPSDriftRule) Check(data mqtt.DeviceData) (bool, string) {
	if r.lastLat == nil {
		r.lastLat = make(map[string]float64)
		r.lastLng = make(map[string]float64)
	}
	prevLat, ok1 := r.lastLat[data.DeviceID]
	prevLng, ok2 := r.lastLng[data.DeviceID]
	r.lastLat[data.DeviceID] = data.Latitude
	r.lastLng[data.DeviceID] = data.Longitude
	if ok1 && ok2 {
		dist := haversine(prevLat, prevLng, data.Latitude, data.Longitude)
		if dist > 100 {
			return true, fmt.Sprintf("无人机 %s GPS漂移: %.1fm", data.DeviceID, dist)
		}
	}
	return false, ""
}

// ==================== 工具函数 ====================

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// haversine 半正矢公式计算两点间距离（米）
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0 // 地球半径（米）
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return R * 2 * math.Asin(math.Sqrt(a))
}
