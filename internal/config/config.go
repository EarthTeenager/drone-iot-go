package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局统一配置结构体
type Config struct {
	Server ServerConfig `yaml:"server"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Redis  RedisConfig  `yaml:"redis"`
	MQTT   MQTTConfig   `yaml:"mqtt"`
}

// ServerConfig HTTP服务端口配置
type ServerConfig struct {
	Port string `yaml:"port"`
}

// MySQLConfig 数据库连接配置
type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
}

// DSN 返回MySQL连接字符串
func (m MySQLConfig) DSN() string {
	return m.User + ":" + m.Password + "@tcp(" + m.Host + ":" + m.Port + ")/" + m.DBName + "?charset=utf8mb4&parseTime=True&loc=Local"
}

// RedisConfig Redis连接配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// MQTTConfig EMQX MQTT连接配置
type MQTTConfig struct {
	Broker   string `yaml:"broker"`
	ClientID string `yaml:"client_id"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Load 加载配置文件，优先读取环境变量覆盖
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// 环境变量覆盖配置文件，方便Docker部署
	if v := os.Getenv("SERVER_PORT"); v != "" {
		cfg.Server.Port = v
	}
	if v := os.Getenv("MYSQL_HOST"); v != "" {
		cfg.MySQL.Host = v
	}
	if v := os.Getenv("MYSQL_PORT"); v != "" {
		cfg.MySQL.Port = v
	}
	if v := os.Getenv("MYSQL_USER"); v != "" {
		cfg.MySQL.User = v
	}
	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
		cfg.MySQL.Password = v
	}
	if v := os.Getenv("MYSQL_DBNAME"); v != "" {
		cfg.MySQL.DBName = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("MQTT_BROKER"); v != "" {
		cfg.MQTT.Broker = v
	}

	return cfg, nil
}

// DefaultConfig 返回本地开发默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{Port: "8080"},
		MySQL: MySQLConfig{
			Host:     "127.0.0.1",
			Port:     "3306",
			User:     "root",
			Password: "root123",
			DBName:   "drone_iot",
		},
		Redis: RedisConfig{
			Addr:     "127.0.0.1:6379",
			Password: "",
			DB:       0,
		},
		MQTT: MQTTConfig{
			Broker:   "tcp://127.0.0.1:1883",
			ClientID: "drone-server",
			Username: "admin",
			Password: "public",
		},
	}
}
