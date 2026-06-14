package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

// Recovery 全局Panic Recovery中间件，确保无panic崩溃
// JD-任职技能2：全局异常封装齐全
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[PANIC] 请求 %s %s 发生panic: %v\n堆栈: %s\n",
					c.Request.Method, c.Request.URL.Path, err, string(debug.Stack()))
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "服务器内部错误",
					"data":    nil,
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}

// RequestLogger 请求日志中间件，记录耗时、IP、路径
// JD-任职技能2：Gin中间件完整落地
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		ip := c.ClientIP()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		log.Printf("[HTTP] %s | %3d | %10v | %s | %s",
			ip, status, latency, c.Request.Method, path)
	}
}

// CORS 跨域中间件，允许前端HTML页面访问后端API和WebSocket
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
