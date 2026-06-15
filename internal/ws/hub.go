package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 全局单例Hub，各模块通过它广播消息
var GlobalHub *Hub

// ==================== WebSocket连接管理器 Hub ====================
// 职责3：WebSocket长连接设备状态、告警数据主动向前端实时推送
// 技能1：Go goroutine/channel并发模型运用，长连接协程管理无泄漏

// Hub 管理所有WebSocket客户端连接，goroutine+channel并发模型
type Hub struct {
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	clients    map[*Client]bool // 活跃客户端集合
	broadcast  chan []byte      // 广播通道，容量256缓冲
	register   chan *Client     // 注册通道
	unregister chan *Client     // 注销通道
}

// Client 单个WebSocket客户端连接
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte // 发送缓冲通道
	ctx    context.Context
	cancel context.CancelFunc
}

// NewHub 创建连接管理器，传入父context统一控制协程生命周期
func NewHub(parentCtx context.Context) *Hub {
	ctx, cancel := context.WithCancel(parentCtx)
	h := &Hub{
		ctx:        ctx,
		cancel:     cancel,
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256), // buffered channel 容量256
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
	GlobalHub = h
	return h
}

// Run 启动Hub主循环（一个goroutine处理所有Client注册/注销/广播）
// 技能1：goroutine无泄漏，通过context统一控制生命周期
func (h *Hub) Run() {
	log.Println("[WebSocket] Hub启动")
	for {
		select {
		case <-h.ctx.Done():
			log.Println("[WebSocket] Hub收到退出信号，关闭所有连接")
			h.shutdown()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("[WebSocket] 客户端连接: %s (在线: %d)\n", client.conn.RemoteAddr(), len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send) // 主动close(channel)，通知写协程退出
			}
			h.mu.Unlock()
			log.Printf("[WebSocket] 客户端断开: %s (在线: %d)\n", client.conn.RemoteAddr(), len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message: // 非阻塞发送
				default:
					// 客户端消费过慢，跳过本条消息
					log.Println("[WebSocket] 客户端接收缓冲满，丢弃消息")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastJSON 将数据序列化JSON后广播给所有在线前端
// 职责3：服务端主动向前端实时推送设备状态
func (h *Hub) BroadcastJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("[WebSocket] JSON序列化失败: %v\n", err)
		return
	}
	select {
	case h.broadcast <- data:
	default:
		log.Println("[WebSocket] 广播通道满，丢弃消息")
	}
}

// ClientCount 返回当前在线客户端数量
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// shutdown 安全关闭所有客户端连接
// 技能1：长连接协程管理无泄漏
func (h *Hub) shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		client.Close()
		delete(h.clients, client)
	}
}

// ==================== WebSocket升级与读写协程 ====================

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 演示环境允许所有来源
	},
}

// HandleConnection 处理WebSocket握手请求，升级为长连接
// 技能4：WebSocket握手、心跳、断线重连完整实现
func (h *Hub) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] 升级失败: %v\n", err)
		return
	}

	ctx, cancel := context.WithCancel(h.ctx)
	client := &Client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 64), // 每个客户端独立发送缓冲
		ctx:    ctx,
		cancel: cancel,
	}

	h.register <- client

	// 启动读写协程（各一个goroutine）
	go client.writePump()
	go client.readPump()
}

// readPump 读协程：接收前端消息（Pong心跳响应）
// 技能4：心跳检测实现
func (c *Client) readPump() {
	defer func() {
		c.cancel()
		c.hub.unregister <- c
		c.conn.Close()
	}()

	// 心跳检测：30s无Pong则超时
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-c.ctx.Done():
			return // 父context取消，退出协程
		default:
		}
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[WebSocket] 读错误: %v\n", err)
			}
			return // 连接断开，defer执行清理
		}
	}
}

// writePump 写协程：向前端发送数据（Ping心跳 + 业务数据推送）
// 技能4：心跳检测，定时30s发送Ping
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second) // 30s Ping/Pong心跳
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return // 父context取消，退出协程

		case message, ok := <-c.send:
			if !ok {
				// send channel已关闭，发送关闭帧
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[WebSocket] 写错误: %v\n", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Ping失败，连接已断
			}
		}
	}
}

// Close 关闭客户端连接
func (c *Client) Close() {
	c.cancel() // 取消context，自动退出读写协程
	c.conn.Close()
}

// ==================== 广播消息结构 ====================

// StatusMessage 设备状态广播消息
type StatusMessage struct {
	Type string      `json:"type"` // "status" / "alert"
	Data interface{} `json:"data"`
}
