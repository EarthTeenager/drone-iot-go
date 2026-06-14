// 无人机硬件模拟器
// 无需真实大疆硬件，模拟多台无人机定时上报飞行数据
// JD-加分项1：无人机设备模拟器，无需真实硬件即可演示设备对接流程
//
// 启动: go run simulator/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// 模拟无人机设备列表
var drones = []struct {
	ID      string
	Name    string
	BaseLat float64 // 基准坐标：成都分公司附近
	BaseLng float64
}{
	{ID: "drone-001", Name: "大疆M300-001", BaseLat: 30.5728, BaseLng: 104.0668},
	{ID: "drone-002", Name: "大疆M300-002", BaseLat: 30.5740, BaseLng: 104.0680},
	{ID: "drone-003", Name: "大疆M300-003", BaseLat: 30.5715, BaseLng: 104.0655},
}

// MQTT模拟数据上报格式
type StatusReport struct {
	DeviceID   string  `json:"device_id"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Altitude   float64 `json:"altitude"`
	Speed      float64 `json:"speed"`
	Battery    int     `json:"battery"`
	Roll       float64 `json:"roll"`
	Pitch      float64 `json:"pitch"`
	Yaw        float64 `json:"yaw"`
	FlightMode string  `json:"flight_mode"`
	GPSStatus  int     `json:"gps_status"`
	Timestamp  int64   `json:"timestamp"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	log.Println("========================================")
	log.Println("  无人机模拟器启动中...")
	log.Println("  (无需真实硬件，模拟大疆无人机飞行数据)")
	log.Println("========================================")

	// 连接EMQX
	opts := mqtt.NewClientOptions().
		AddBroker("tcp://127.0.0.1:1883").
		SetClientID("drone-simulator").
		SetUsername("admin").
		SetPassword("public").
		SetKeepAlive(30 * time.Second).
		SetConnectTimeout(10 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		log.Fatalf("MQTT连接失败: %v (请确保EMQX已通过 docker-compose 启动)\n", token.Error())
	}
	log.Println("[SIM] MQTT连接成功，开始上报无人机数据...")

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 每台无人机一个goroutine独立上报
	for i := range drones {
		drone := drones[i]
		go simulateDrone(client, drone)
	}

	log.Println("[SIM] 3台无人机模拟上报启动完成，Ctrl+C 停止")
	<-quit
	log.Println("[SIM] 模拟器停止")
	client.Disconnect(250)
}

// simulateDrone 模拟单台无人机循环上报飞行数据
func simulateDrone(client mqtt.Client, drone struct {
	ID      string
	Name    string
	BaseLat float64
	BaseLng float64
}) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(3 * time.Second) // 每3秒上报一次
	defer ticker.Stop()

	battery := 100       // 初始满电
	lat := drone.BaseLat // 初始位置
	lng := drone.BaseLng
	alt := 120.0 // 初始高度120m

	for range ticker.C {
		// 模拟飞行数据变化
		lat += (rng.Float64() - 0.5) * 0.0005 // 经纬度小幅漂移
		lng += (rng.Float64() - 0.5) * 0.0005
		alt += (rng.Float64() - 0.5) * 5 // 高度小幅变化
		battery -= rng.Intn(2)           // 电量自然下降
		if battery < 0 {
			battery = 0
		}

		// 每隔20%概率出现异常数据（用于触发告警演示）
		if rng.Float64() < 0.20 {
			if rng.Float64() < 0.5 {
				// 模拟低电量告警
				battery = 15 + rng.Intn(5)
			} else if rng.Float64() < 0.5 {
				// 模拟高度突变告警
				alt += float64(60 + rng.Intn(40))
			} else {
				// 模拟GPS漂移告警
				lat += 0.002
				lng += 0.002
			}
		}

		report := StatusReport{
			DeviceID:   drone.ID,
			Latitude:   round(lat, 6),
			Longitude:  round(lng, 6),
			Altitude:   round(alt, 2),
			Speed:      round(5+rng.Float64()*20, 2),
			Battery:    battery,
			Roll:       round((rng.Float64()-0.5)*20, 2),
			Pitch:      round((rng.Float64()-0.5)*15, 2),
			Yaw:        round(rng.Float64()*360, 2),
			FlightMode: []string{"hover", "cruising", "returning"}[rng.Intn(3)],
			GPSStatus:  rng.Intn(6),
			Timestamp:  time.Now().Unix(),
		}

		data, _ := json.Marshal(report)
		topic := fmt.Sprintf("drone/%s/status", drone.ID)
		token := client.Publish(topic, 1, false, data)
		token.Wait()

		log.Printf("[SIM] %s √ 电量=%d%% 高度=%.1fm 坐标=(%.4f,%.4f) 模式=%s\n",
			drone.ID, report.Battery, report.Altitude,
			report.Latitude, report.Longitude, report.FlightMode)

		// 电量过低时重置（模拟充电/降落），防止始终告警
		if battery < 10 {
			battery = 100
			alt = 120.0
			lat = drone.BaseLat
			lng = drone.BaseLng
			log.Printf("[SIM] %s 电量耗尽，模拟回航充电重置\n", drone.ID)
		}
	}
}

func round(v float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	return float64(int(v*pow)) / pow
}
