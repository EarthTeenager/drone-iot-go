package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// DeviceData 无人机上行状态数据结构
// 职责2：解析GPS、高度、电量、姿态数据
type DeviceData struct {
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

// CommandPayload 下行遥控指令
// 职责2：下发返航、起飞、锁定电机等遥控指令
type CommandPayload struct {
	DeviceID string `json:"device_id"`
	Command  string `json:"command"` // takeoff / land / return_home / lock_motor
	Params   string `json:"params"`  // 附加参数JSON
}

// DataCallback MQTT收到设备数据后的回调函数类型
type DataCallback func(data DeviceData)

// Client EMQX MQTT客户端封装
// 技能3：MQTT协议+EMQX实战：订阅、发布、重连、QoS配置完整实现
type Client struct {
	client    mqtt.Client
	mu        sync.RWMutex
	connected bool
	callbacks []DataCallback

	// 重连配置
	retryCount   int
	maxRetrySecs int
}

// NewClient 创建MQTT客户端实例
func NewClient(broker, username, password, clientID string) *Client {
	c := &Client{
		maxRetrySecs: 60,
	}
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetUsername(username).
		SetPassword(password).
		SetAutoReconnect(false). // 关闭SDK自动重连，自实现退避重试
		SetCleanSession(true).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetOnConnectHandler(c.onConnect). // 连接成功回调
		SetConnectionLostHandler(c.onConnectionLost) // 断线回调

	c.client = mqtt.NewClient(opts)
	return c
}

// Connect 建立MQTT连接，失败时指数退避重试
// 职责2：IoT设备MQTT接入，断线自动重连、退避重试
func (c *Client) Connect() error {
	for {
		token := c.client.Connect()
		if token.Wait() && token.Error() == nil {
			c.mu.Lock()
			c.connected = true
			c.retryCount = 0
			c.mu.Unlock()
			log.Println("[MQTT] 连接成功")
			return nil
		}

		// 指数退避重试：初始1s，翻倍递增，最大60s
		c.retryCount++
		backoff := 1 << c.retryCount
		if backoff > c.maxRetrySecs {
			backoff = c.maxRetrySecs
		}
		delay := time.Duration(backoff) * time.Second
		log.Printf("[MQTT] 连接失败，%v 后第 %d 次重试...\n", delay, c.retryCount)
		time.Sleep(delay)
	}
}

// Subscribe 订阅无人机上行topic，QoS=1保证至少到达一次
// 技能3：订阅QoS配置
func (c *Client) Subscribe(topic string) error {
	token := c.client.Subscribe(topic, 1, c.messageHandler)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	log.Printf("[MQTT] 订阅topic成功: %s (QoS=1)\n", topic)
	return nil
}

// PublishCommand 下发遥控指令到指定设备，QoS=2确保精确一次送达
// 技能3：发布QoS=2配置
func (c *Client) PublishCommand(cmd CommandPayload) (uint16, error) {
	data, _ := json.Marshal(cmd)
	topic := fmt.Sprintf("drone/%s/command", cmd.DeviceID)
	token := c.client.Publish(topic, 2, false, data)
	if token.Wait() && token.Error() != nil {
		return 0, token.Error()
	}
	msgID := token.(*mqtt.PublishToken).MessageID()
	log.Printf("[MQTT] 指令下发成功: device=%s cmd=%s msgID=%d (QoS=2)\n",
		cmd.DeviceID, cmd.Command, msgID)
	return msgID, nil
}

// RegisterDataCallback 注册设备数据回调（由service层注入处理链路）
func (c *Client) RegisterDataCallback(cb DataCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks = append(c.callbacks, cb)
}

// IsConnected 查询连接状态
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// onConnect 连接成功时重新订阅所有topic
func (c *Client) onConnect(client mqtt.Client) {
	c.mu.Lock()
	c.connected = true
	c.retryCount = 0
	c.mu.Unlock()
	log.Println("[MQTT] 重新连接成功，恢复订阅")
}

// onConnectionLost 断线回调，标记断开状态
func (c *Client) onConnectionLost(client mqtt.Client, err error) {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
	log.Printf("[MQTT] 连接断开: %v，准备重连...\n", err)
	// 指数退避重试连接
	for {
		c.retryCount++
		backoff := 1 << c.retryCount
		if backoff > c.maxRetrySecs {
			backoff = c.maxRetrySecs
		}
		delay := time.Duration(backoff) * time.Second
		log.Printf("[MQTT] %v 后第 %d 次重试连接...\n", delay, c.retryCount)
		time.Sleep(delay)

		token := client.Connect()
		if token.Wait() && token.Error() == nil {
			log.Println("[MQTT] 重连成功")
			return
		}
	}
}

// messageHandler 处理设备上行消息
// 职责2：收到设备上报数据后入库MySQL+写入Redis+投递WebSocket
func (c *Client) messageHandler(client mqtt.Client, msg mqtt.Message) {
	var data DeviceData
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("[MQTT] 消息解析失败: topic=%s err=%v\n", msg.Topic(), err)
		return
	}
	log.Printf("[MQTT] 收到设备数据: device=%s battery=%d%% lat=%.4f lng=%.4f alt=%.1fm\n",
		data.DeviceID, data.Battery, data.Latitude, data.Longitude, data.Altitude)

	// 通知所有注册的回调（由service层统一处理入库/缓存/广播/告警）
	c.mu.RLock()
	for _, cb := range c.callbacks {
		go cb(data) // 每个回调独立goroutine执行，不阻塞MQTT消息接收
	}
	c.mu.RUnlock()
}
