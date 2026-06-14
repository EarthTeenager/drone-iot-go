package repository

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

var RDB *redis.Client

// InitRedis 初始化Redis连接
func InitRedis(addr, password string, db int) error {
	RDB = redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     100, // 连接池大小，适配高并发设备接入
		MinIdleConns: 10,  // 最小空闲连接
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := RDB.Ping(ctx).Err(); err != nil {
		return err
	}
	log.Println("[Redis] 连接成功")
	return nil
}

// SetDeviceOnline 设置设备在线状态到Redis缓存（Hash结构）
// JD-任职技能5：Redis内存模型、热点设备状态缓存
// Key: device:{deviceID} Hash字段: status/last_seen/latitude/longitude/altitude/battery
func SetDeviceOnline(ctx context.Context, deviceID string, status map[string]interface{}) error {
	key := fmt.Sprintf("device:%s", deviceID)
	// 批量写入多个字段
	return RDB.HSet(ctx, key, status).Err()
}

// GetDeviceOnline 查询设备在线状态（Hash所有字段）
func GetDeviceOnline(ctx context.Context, deviceID string) (map[string]string, error) {
	key := fmt.Sprintf("device:%s", deviceID)
	return RDB.HGetAll(ctx, key).Result()
}

// SetDeviceStatusTTL 设置设备状态Key过期时间
// 设备状态TTL 30s：持续上报则持续刷新，停止上报则自动视为离线
func SetDeviceStatusTTL(ctx context.Context, deviceID string) error {
	key := fmt.Sprintf("device:%s", deviceID)
	return RDB.Expire(ctx, key, 30*time.Second).Err()
}

// TryLock 分布式锁：SET NX + Lua脚本原子释放，防止重复下发遥控指令
// JD-岗位职责5：Redis分布式锁实现
func TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return RDB.SetNX(ctx, key, "locked", ttl).Result()
}

// Unlock 原子释放锁（Lua脚本保证原子性）
func Unlock(ctx context.Context, key string) error {
	// Lua脚本：只有锁持有者才能释放，防止误删其他客户端的锁
	script := `
		if redis.call("GET", KEYS[1]) == "locked" then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	return RDB.Eval(ctx, script, []string{key}).Err()
}

// SetSession 存储WebSocket会话
// JD-岗位职责5：Redis会话管理
func SetSession(ctx context.Context, sessionID string, data string) error {
	return RDB.Set(ctx, fmt.Sprintf("session:%s", sessionID), data, 1*time.Hour).Err()
}

// GetSession 获取会话信息
func GetSession(ctx context.Context, sessionID string) (string, error) {
	return RDB.Get(ctx, fmt.Sprintf("session:%s", sessionID)).Result()
}

// DeleteSession 删除过期会话
func DeleteSession(ctx context.Context, sessionID string) error {
	return RDB.Del(ctx, fmt.Sprintf("session:%s", sessionID)).Err()
}

// PushAlert 告警历史记录到Redis List，保留最近100条
func PushAlert(ctx context.Context, alertJSON string) error {
	key := "alerts:latest"
	pipe := RDB.Pipeline()
	pipe.LPush(ctx, key, alertJSON)
	pipe.LTrim(ctx, key, 0, 99) // 仅保留最近100条
	_, err := pipe.Exec(ctx)
	return err
}

// GetAlerts 查询最近N条告警
func GetAlerts(ctx context.Context, limit int64) ([]string, error) {
	return RDB.LRange(ctx, "alerts:latest", 0, limit-1).Result()
}
