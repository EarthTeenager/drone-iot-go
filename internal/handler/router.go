package handler

import (
	"drone-iot-demo/internal/middleware"
	"drone-iot-demo/internal/service"
	"drone-iot-demo/internal/ws"

	"github.com/gin-gonic/gin"
)

// SetupRouter 注册Gin路由、中间件、WebSocket端点
// JD-岗位职责1：Gin分层架构设计，路由分组，可拆分微服务
// JD-任职技能2：路由分组、中间件、全局异常封装齐全
func SetupRouter(ds *service.DeviceService) *gin.Engine {
	r := gin.New()

	// 全局中间件链
	r.Use(middleware.Recovery())      // Panic捕获恢复
	r.Use(middleware.RequestLogger()) // 请求日志
	r.Use(middleware.CORS())          // 跨域支持

	// WebSocket端点
	r.GET("/ws", func(c *gin.Context) {
		if ws.GlobalHub != nil {
			ws.GlobalHub.HandleConnection(c.Writer, c.Request)
		}
	})

	// 静态文件（前端监控页面）
	r.Static("/static", "./static")

	// API v1 路由分组，每组独立Handler文件，方便后续微服务拆分
	// JD-岗位职责1：可拆分微服务，支撑高并发低延迟场景
	v1 := r.Group("/api/v1")
	{
		deviceHandler := NewDeviceHandler(ds)

		// 设备管理路由组（可独立拆为设备微服务）
		v1.GET("/devices", deviceHandler.ListDevices)

		// 指令下发路由组（可独立拆为指令微服务）
		v1.POST("/commands", deviceHandler.SendCommand)

		// 历史数据路由组（可独立拆为数据查询微服务）
		v1.GET("/history/:device_id", deviceHandler.GetDeviceHistory)
	}

	return r
}
