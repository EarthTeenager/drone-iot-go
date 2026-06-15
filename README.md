# drone-iot-demo 无人机IoT实时监控系统

> 
>
> 完整演示链路：**中间件 → 后端 → 模拟器 → Web页面 → 指令下发双向通信**

## 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 后端框架 | Go 1.21 + Gin | RESTful API、路由分组、中间件 |
| 数据库 | MySQL 8.0 + GORM | 3张核心表、索引优化、慢SQL治理 |
| 缓存 | Redis 7 | 设备状态缓存(Hash)、分布式锁(SET NX+Lua)、会话管理 |
| 消息中间件 | EMQX 5 (MQTT) | QoS配置、设备上行/指令下行、指数退避重连 |
| 实时通信 | WebSocket | Hub连接管理器、心跳Ping/Pong(30s)、断线重连 |
| 并发模型 | goroutine + channel + context | 协程生命周期统一管理、无泄漏 |
| 部署 | Docker Compose | 一键启动 MySQL + Redis + EMQX |

## 目录结构

```
drone-iot-demo/
├── cmd/server/main.go          # 后端入口，启动所有模块
├── simulator/main.go           # 无人机硬件模拟器（无需真机）
├── internal/
│   ├── config/config.go        # 统一配置加载 + 环境变量覆盖
│   ├── model/                   # 3张核心表GORM模型定义
│   │   ├── drone.go            # 设备表 drones
│   │   ├── device_status.go    # 实时状态表 device_status
│   │   └── command_log.go      # 指令记录表 command_logs
│   ├── repository/             # 数据持久层
│   │   ├── database.go         # GORM初始化 + 慢SQL监控
│   │   ├── drone_repo.go       # 设备CRUD
│   │   ├── device_status_repo.go # 状态历史查询
│   │   ├── command_log_repo.go # 指令日志CRUD
│   │   └── redis.go            # Redis缓存、分布式锁、会话管理
│   ├── mqtt/client.go          # EMQX MQTT客户端、QoS、重连
│   ├── ws/hub.go               # WebSocket Hub、心跳、协程管理
│   ├── middleware/middleware.go # Gin中间件(Recovery/日志/CORS)
│   ├── service/
│   │   ├── device_service.go   # 核心业务：数据→入库+缓存+广播+告警
│   │   ├── alert_service.go    # 告警引擎：3条规则+AlertRule接口
│   │   └── errors.go           # 业务错误定义
│   └── handler/
│       ├── router.go           # Gin路由注册 + WebSocket端点
│       └── device_handler.go   # API处理器(设备/指令/历史查询)
├── static/monitor.html         # 前端实时监控页面（纯HTML）
├── config.yaml                 # 本地开发配置文件
├── docker-compose.yml          # 一键启动中间件
└── docs/architecture.md        # 高可用架构与集群部署方案
```

## 快速启动

### 步骤1：启动中间件（约30秒）
```bash
docker-compose up -d
# 等待 MySQL、Redis、EMQX 状态变为 healthy
docker-compose ps
```

### 步骤2：启动后端服务
```bash
# 安装依赖
go mod tidy

# 启动后端（自动建表、连接MQTT/Redis/MySQL）
go run cmd/server/main.go
```

### 步骤3：启动无人机模拟器（新终端）
```bash
go run simulator/main.go
# 3台无人机每3秒上报一次飞行数据（含随机异常触发告警）
```

### 步骤4：打开监控页面
浏览器访问：**http://localhost:8080/static/monitor.html**

你将看到：
- 3台无人机实时状态卡片（坐标、高度、电量、飞行模式）持续刷新
- **实时告警弹窗**：低电量 / 高度突变 / GPS漂移
- WebSocket长连接推送，延迟 < 100ms

### 步骤5：验证双向通信
在监控页面底部**遥控指令面板**：
- 选择设备 → 点击"起飞/返航/降落/锁定电机"
- 指令通过 MQTT(QoS=2) 下发到设备
- 后端记录指令日志到 `command_logs` 表

## API接口

| 方法 | 路径 | 说明 | JD对标 |
|------|------|------|--------|
| GET | `/api/v1/devices` | 设备列表 | RESTful API |
| POST | `/api/v1/commands` | 下发遥控指令 | IoT指令下行 |
| GET | `/api/v1/history/:device_id` | 设备历史数据 | 历史数据查询 |
| WS | `/ws` | WebSocket连接 | 实时数据推送 |
| GET | `/static/monitor.html` | 监控页面 | 前端演示 |

统一返回格式：`{ "code": 0, "message": "success", "data": {...} }`

## EMQX Dashboard

管理后台：http://localhost:18083 (admin/public)
- 查看MQTT连接状态
- 监控消息吞吐量
- 验证QoS配置

## 面试演示验证清单

- [ ] Docker Compose 3个中间件全部 healthy
- [ ] go run cmd/server/main.go 无报错启动
- [ ] 启动模拟器后，监控页面3台无人机状态卡片实时刷新
- [ ] 电量 < 20% 时触发低电量告警（前端弹窗+Redis记录）
- [ ] 指令下发面板 → API → MQTT → command_logs 表完整链路
- [ ] WebSocket心跳：30s无数据自动落Ping/Pong
- [ ] 断线重连：关闭EMQX → 日志显示重试 → 重新连接
- [ ] 慢SQL日志：GORM控制台输出 >200ms的SQL
- [ ] Redis中 `KEYS device:*` 查看设备缓存
- [ ] EMQX Dashboard http://localhost:18083 查看MQTT连接

## 生产级细节（JD hard constraints）

| 特性 | 实现位置 | 描述 |
|------|---------|------|
| MQTT指数退避重连 | `internal/mqtt/client.go:72` | 1s→2s→4s...→60s |
| QoS配置 | `internal/mqtt/client.go:85,94` | 订阅QoS=1，下发QoS=2 |
| 慢SQL治理 | `internal/repository/database.go:28` | 阈值200ms，Explain分析 |
| 分布式锁 | `internal/repository/redis.go:55` | SET NX + Lua原子释放 |
| WebSocket心跳 | `internal/ws/hub.go:183` | 30s Ping/Pong + 60s超时 |
| goroutine防泄漏 | `internal/ws/hub.go:47` | context统一控制生命周期 |
| 告警规则接口化 | `internal/service/alert_service.go:23` | AlertRule interface可扩展 |
| 统一异常捕获 | `internal/middleware/middleware.go:13` | Panic Recovery中间件 |
