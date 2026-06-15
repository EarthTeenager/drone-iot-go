package repository

import (
	"log"
	"time"

	"drone-iot-demo/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB 初始化MySQL连接、GORM自动迁移建表、配置慢SQL日志
// 技能5：MySQL索引设计、GORM复杂查询示例
func InitDB(dsn string) error {
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		// 慢SQL日志：阈值200ms，输出执行计划和SQL语句
		// 职责4：慢SQL治理方案落地，使用Explain分析执行计划
		Logger: logger.New(
			log.Default(),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond, // 慢SQL阈值
				LogLevel:                  logger.Warn,            // WARN级别输出慢查询
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		),
	})
	if err != nil {
		return err
	}

	// 自动迁移3张核心表
	if err := DB.AutoMigrate(
		&model.Drone{},
		&model.DeviceStatus{},
		&model.CommandLog{},
	); err != nil {
		return err
	}

	// GORM Logger已配置SlowThreshold=200ms自动输出慢SQL（见上方Logger配置）
	// 生产环境可结合该日志输出自动执行Explain分析执行计划

	return nil
}
