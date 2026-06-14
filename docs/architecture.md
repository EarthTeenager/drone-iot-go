# 高可用架构与集群部署方案

## 一、高可用架构设计（对标岗位职责7）

### 整体拓扑

```
                   ┌──────────────┐
                   │  Nginx LB     │  (负载均衡)
                   │  (Ingress)   │
                   └──┬───┬───┬───┘
                      │   │   │
         ┌────────────┼───┼───┼────────────┐
         ▼            ▼   ▼   ▼            ▼
   ┌──────────┐ ┌──────────┐ ┌──────────┐
   │ Server-1 │ │ Server-2 │ │ Server-N │  (后端服务集群)
   │ Gin+WS   │ │ Gin+WS   │ │ Gin+WS   │
   └────┬─────┘ └────┬─────┘ └────┬─────┘
        │            │            │
        └────────────┼────────────┘
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
   ┌──────────┐ ┌──────────┐ ┌──────────┐
   │ EMQX-1   │ │ EMQX-2   │ │ EMQX-3   │  (EMQX集群)
   │ (Node)   │─│ (Node)   │─│ (Node)   │
   └──────────┘ └──────────┘ └──────────┘
        │            │            │
        └────────────┼────────────┘
                     │  ▲
                     ▼  │ 设备上行/指令下行
              ┌──────────────┐
              │  无人机设备   │
              └──────────────┘
```

### 数据存储层

```
┌─────────────────────────────────────┐
│ MySQL 主从架构                        │
│ ┌───────────┐    ┌───────────┐       │
│ │ Master    │───▶│ Slave-1   │       │
│ │ (写)      │    │ (读)      │       │
│ └───────────┘    └───────────┘       │
│                   ┌───────────┐       │
│                   │ Slave-2   │       │
│                   │ (读)      │       │
│                   └───────────┘       │
│ GORM DBResolver 实现读写分离            │
└─────────────────────────────────────┘

┌─────────────────────────────────────┐
│ Redis Sentinel 哨兵模式                │
│ ┌───────────┐ ┌───────────┐         │
│ │ Master    │ │ Sentinel  │         │
│ │ (R/W)     │─│ x3 (监控)  │         │
│ └───────────┘ └───────────┘         │
│ ┌───────────┐ ┌───────────┐         │
│ │ Slave-1   │ │ Slave-2   │         │
│ └───────────┘ └───────────┘         │
│ 自动故障转移 + 读写分离                 │
└─────────────────────────────────────┘
```

## 二、数据一致性保障方案

### 设备状态一致性
```
MySQL(Primary) ←── 写入设备状态(T+0ms)
                      │
Redis ←──────────────┘ 同步写缓存(T+10ms)
                      │
WebSocket Broadcast ←┘ 推送前端(T+50ms)
```

策略：
1. **先写MySQL**（持久化保证），再写Redis（可降级）
2. Redis写入失败仅日志告警，不阻塞主流程
3. 前端定时轮询（60s）Redis最新状态，兜底WebSocket可能的消息丢失

### 指令下发幂等性
- 分布式锁（Redis SET NX）：同一设备+同一指令5s内不可重复
- command_logs 表 device_id+command+create_time 联合约束防止重复记录
- MQTT QoS=2 保证精确一次送达

### 数据一致性监控
- 定时任务：每5分钟对比 Redis在线设备数 vs MySQL最近1分钟有上报的设备数
- 偏差超过10%时触发一致性告警

## 三、水平扩展策略

### 后端服务无状态化设计
- 所有状态存Redis（设备在线、会话），后端服务本身无状态
- 新实例启动即加入Nginx upstream，无需特殊编排
- WebSocket需要 sticky session（Nginx ip_hash）保证同一客户端始终路由到同一实例

### 数据库水平扩展
- **垂直拆分**：设备表、状态表、日志表已在不同表，可按业务进一步分库
- **水平分片**：按 device_id 哈希取模分表（如 device_status_00 ~ device_status_15）
- **冷热分离**：30天内状态数据查MySQL，30天前归档到对象存储或ES

### EMQX集群水平扩展
- EMQX 5.x 原生支持集群（Mnesia/etcd）
- 新增节点自动加入集群
- 后端通过HAProxy/TCP LB连接EMQX集群的任意节点

## 四、高可用关键指标

| 组件 | 故障场景 | 恢复策略 | RTO | RPO |
|------|---------|---------|-----|-----|
| MySQL Master | 宕机 | MHA自动切换Slave为Master | <30s | 0 |
| Redis Master | 宕机 | Sentinel自动选举新Master | <10s | 0 |
| EMQX Node | 宕机 | 设备自动重连到其他集群Node | <5s | 0 |
| Gin Server | 宕机 | Nginx health check剔除 | <5s | 0 |

## 五、当前Demo阶段简化说明

本Demo为单机部署模式方便面试演示，生产环境下上述架构通过以下方式启用：

1. **MySQL读写分离**：GORM DBResolver插件配置多数据源
2. **Redis Sentinel**：go-redis支持哨兵模式，修改连接地址为 sentinel:// 即可
3. **EMQX集群**：docker-compose中增加 emqx2/emqx3 服务节点
4. **Nginx负载均衡**：增加 nginx.conf + docker-compose Nginx服务
5. **监控告警**：Prometheus + Grafana (EMQX自带/metrics端点)

### 快速启用生产模式
```bash
# 生产版 docker-compose
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
# docker-compose.prod.yml 会增加:
#   - Nginx:8080 (lb)
#   - MySQL Slave x2
#   - Redis Sentinel x3
#   - EMQX Node x2
#   - Gin Server replica=2
```
