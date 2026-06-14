// drone-iot-demo 无人机IoT实时监控Go后端
// 对标私迪航空 Go后端工程师 JD全部职责与技能要求
//
// 启动方式:
//
//  1. docker-compose up -d         # 启动MySQL/Redis/EMQX中间件
//  2. go run cmd/server/main.go     # 启动后端服务
//  3. go run simulator/main.go      # 启动无人机模拟器
//  4. 浏览器打开 http://localhost:8080/static/monitor.html
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"drone-iot-demo/internal/config"
	"drone-iot-demo/internal/handler"
	"drone-iot-demo/internal/mqtt"
	"drone-iot-demo/internal/repository"
	"drone-iot-demo/internal/service"
	"drone-iot-demo/internal/ws"

	"github.com/gin-gonic/gin"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("========================================")
	log.Println("  无人机IoT实时监控后端服务启动中...")
	log.Println("  drone-iot-demo v1.0")
	log.Println("========================================")

	// ===== 1. 加载配置 =====
	cfg := config.DefaultConfig()
	if _, err := os.Stat("config.yaml"); err == nil {
		c, err := config.Load("config.yaml")
		if err != nil {
			log.Printf("配置文件加载失败，使用默认配置: %v\n", err)
		} else {
			cfg = c
		}
	}
	log.Printf("[Config] 服务端口: %s\n", cfg.Server.Port)

	// ===== 2. 初始化MySQL =====
	if err := repository.InitDB(cfg.MySQL.DSN()); err != nil {
		log.Printf("[MySQL] 连接失败: %v (继续运行，DB功能不可用)\n", err)
	} else {
		log.Println("[MySQL] 数据库连接成功，表迁移完成")
	}

	// ===== 3. 初始化Redis =====
	if err := repository.InitRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB); err != nil {
		log.Printf("[Redis] 连接失败: %v (继续运行，缓存功能不可用)\n", err)
	} else {
		log.Println("[Redis] 连接成功")
	}

	// ===== 4. 初始化WebSocket Hub =====
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := ws.NewHub(ctx)
	go hub.Run()
	log.Printf("[WebSocket] Hub启动，广播通道容量=%d\n", 256)

	// ===== 5. 初始化MQTT客户端 =====
	mqttCli := mqtt.NewClient(cfg.MQTT.Broker, cfg.MQTT.Username, cfg.MQTT.Password, cfg.MQTT.ClientID)
	go func() {
		if err := mqttCli.Connect(); err != nil {
			log.Printf("[MQTT] 连接失败: %v\n", err)
		}
	}()
	// 等待MQTT连接成功后订阅所有设备上行topic
	go func() {
		for !mqttCli.IsConnected() {
		}
		if err := mqttCli.Subscribe("drone/+/status"); err != nil {
			log.Printf("[MQTT] 订阅失败: %v\n", err)
		}
	}()
	log.Println("[MQTT] 客户端已创建，等待连接...")

	// ===== 6. 初始化告警引擎 =====
	alertService := service.NewAlertService()
	log.Printf("[Alert] 告警引擎启动，已注册3条规则\n")

	// ===== 7. 初始化Service层 =====
	droneRepo := repository.NewDroneRepo()
	statusRepo := repository.NewDeviceStatusRepo()
	cmdLogRepo := repository.NewCommandLogRepo()
	deviceService := service.NewDeviceService(droneRepo, statusRepo, cmdLogRepo, mqttCli)

	// 注册MQTT数据回调：设备上行数据 → 入库 + 缓存 + 广播 + 告警
	mqttCli.RegisterDataCallback(func(data mqtt.DeviceData) {
		deviceService.HandleDeviceData(data)
	})

	// ===== 8. 启动Gin HTTP服务 =====
	gin.SetMode(gin.ReleaseMode)
	router := handler.SetupRouter(deviceService)

	// 优雅关闭：监听系统信号
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("收到信号 %v，正在优雅关闭...\n", sig)
		cancel() // 关闭WebSocket Hub，所有协程通过context退出
		os.Exit(0)
	}()

	// 启动首页重定向
	router.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/static/monitor.html")
	})

	log.Printf("========================================\n")
	log.Printf("  服务已启动: http://localhost:%s\n", cfg.Server.Port)
	log.Printf("  监控页面: http://localhost:%s/static/monitor.html\n", cfg.Server.Port)
	log.Printf("  WebSocket: ws://localhost:%s/ws\n", cfg.Server.Port)
	log.Printf("  API接口: http://localhost:%s/api/v1\n", cfg.Server.Port)
	log.Printf("========================================\n")

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("服务启动失败: %v\n", err)
	}

	_ = alertService // 保持告警引擎生命周期
}
