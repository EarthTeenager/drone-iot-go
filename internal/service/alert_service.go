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

// ==================== 告警规则接口 & 引擎（原有逻辑不变） ====================
type AlertRule interface {
	Name() string
	Check(data mqtt.DeviceData) (bool, string)
}

type AlertService struct {
	mu        sync.RWMutex
	rules     []AlertRule
	lastAlert map[string]time.Time // 告警冷却，量小保留单锁即可
}

func NewAlertService() *AlertService {
	as := &AlertService{
		rules:     make([]AlertRule, 0),
		lastAlert: make(map[string]time.Time),
	}
	as.RegisterRule(&LowBatteryRule{})
	as.RegisterRule(&NewAltitudeChangeRule{}) // 使用分片锁版本
	as.RegisterRule(&NewGPSDriftRule{})       // 使用分片锁版本
	AlertEngine = as
	return as
}

func (as *AlertService) RegisterRule(rule AlertRule) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.rules = append(as.rules, rule)
	log.Printf("[Alert] 规则已注册: %s\n", rule.Name())
}

func (as *AlertService) Check(data mqtt.DeviceData) {
	as.mu.RLock()
	rules := as.rules
	as.mu.RUnlock()

	for _, rule := range rules {
		hit, msg := rule.Check(data)
		if hit {
			coolKey := data.DeviceID + ":" + rule.Name()
			as.mu.Lock()
			lastTime, exists := as.lastAlert[coolKey]
			if exists && time.Since(lastTime) < 30*time.Second {
				as.mu.Unlock()
				continue
			}
			as.lastAlert[coolKey] = time.Now()
			as.mu.Unlock()

			alertMsg := map[string]interface{}{
				"type":      "alert",
				"device_id": data.DeviceID,
				"rule":      rule.Name(),
				"message":   msg,
				"timestamp": time.Now().Unix(),
			}
			log.Printf("[Alert] 告警触发: device=%s rule=%s msg=%s\n",
				data.DeviceID, rule.Name(), msg)

			if ws.GlobalHub != nil {
				ws.GlobalHub.BroadcastJSON(alertMsg)
			}

			alertJSON := fmt.Sprintf(
				`{"device_id":"%s","rule":"%s","message":"%s","timestamp":%d}`,
				data.DeviceID, rule.Name(), msg, time.Now().Unix(),
			)
			repository.PushAlert(context.Background(), alertJSON)
		}
	}
}

// ==================== 低电量规则（无状态，无需改） ====================
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

// ==================== 通用分片基础结构 ====================
const defaultShardCount = 32 // 分片数，可根据并发量调整 16/32/64

// Shard 单个分片：读写锁 + 数据map
type float64Shard struct {
	mu   sync.RWMutex
	data map[string]float64
}

// 字符串简易哈希，用于 deviceID 路由分片
func hashDeviceID(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// getShardIndex 根据 deviceID 获取分片下标
func getShardIndex(deviceID string, shardCnt int) int {
	return int(hashDeviceID(deviceID) % uint64(shardCnt))
}

// ==================== 改造1：高度突变告警 - 分片锁版本 ====================
type NewAltitudeChangeRule struct {
	shards []float64Shard // 分片数组
}

func (r *NewAltitudeChangeRule) Name() string {
	return "高度突变告警"
}

func (r *NewAltitudeChangeRule) Check(data mqtt.DeviceData) (bool, string) {
	// 延迟初始化分片
	if r.shards == nil {
		r.shards = make([]float64Shard, defaultShardCount)
		for i := range r.shards {
			r.shards[i].data = make(map[string]float64)
		}
	}

	idx := getShardIndex(data.DeviceID, defaultShardCount)
	shard := &r.shards[idx]

	// 读历史高度
	shard.mu.RLock()
	prev, exists := shard.data[data.DeviceID]
	shard.mu.RUnlock()

	// 更新当前高度
	shard.mu.Lock()
	shard.data[data.DeviceID] = data.Altitude
	shard.mu.Unlock()

	// 判断高度突变
	if exists && abs(data.Altitude-prev) > 50 {
		return true, fmt.Sprintf("无人机 %s 高度突变: %.1fm → %.1fm", data.DeviceID, prev, data.Altitude)
	}
	return false, ""
}

// ==================== 改造2：GPS漂移告警 - 分片锁版本 ====================
// GPS单分片：同时存纬度、经度
type gpsShard struct {
	mu      sync.RWMutex
	lastLat map[string]float64
	lastLng map[string]float64
}

type NewGPSDriftRule struct {
	shards []gpsShard
}

func (r *NewGPSDriftRule) Name() string {
	return "GPS漂移告警"
}

func (r *NewGPSDriftRule) Check(data mqtt.DeviceData) (bool, string) {
	// 延迟初始化分片
	if r.shards == nil {
		r.shards = make([]gpsShard, defaultShardCount)
		for i := range r.shards {
			r.shards[i].lastLat = make(map[string]float64)
			r.shards[i].lastLng = make(map[string]float64)
		}
	}

	idx := getShardIndex(data.DeviceID, defaultShardCount)
	shard := &r.shards[idx]

	shard.mu.RLock()
	prevLat, ok1 := shard.lastLat[data.DeviceID]
	prevLng, ok2 := shard.lastLng[data.DeviceID]
	shard.mu.RUnlock()

	// 更新最新坐标
	shard.mu.Lock()
	shard.lastLat[data.DeviceID] = data.Latitude
	shard.lastLng[data.DeviceID] = data.Longitude
	shard.mu.Unlock()

	if ok1 && ok2 {
		dist := haversine(prevLat, prevLng, data.Latitude, data.Longitude)
		if dist > 100 {
			return true, fmt.Sprintf("无人机 %s GPS漂移: %.1fm", data.DeviceID, dist)
		}
	}
	return false, ""
}

// ==================== 工具函数（保留原有） ====================
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
