package handler

import (
	"net/http"

	"drone-iot-demo/internal/model"
	"drone-iot-demo/internal/mqtt"
	"drone-iot-demo/internal/service"

	"github.com/gin-gonic/gin"
)

// DeviceHandler 设备管理接口处理器
// JD-岗位职责6：设备管理接口，统一返回JSON格式
type DeviceHandler struct {
	deviceService *service.DeviceService
}

// NewDeviceHandler 创建设备Handler
func NewDeviceHandler(ds *service.DeviceService) *DeviceHandler {
	return &DeviceHandler{deviceService: ds}
}

// ==================== 统一响应结构 ====================
// JD-岗位职责6：统一接口返回结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func Ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "success", Data: data})
}

func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{Code: code, Message: msg, Data: nil})
}

// ==================== 设备管理接口 ====================

// ListDevices GET /api/v1/devices 获取设备列表
func (h *DeviceHandler) ListDevices(c *gin.Context) {
	devices, err := h.deviceService.GetDeviceList()
	if err != nil {
		Fail(c, 500, "查询设备列表失败: "+err.Error())
		return
	}
	if devices == nil {
		devices = []model.Drone{}
	}
	Ok(c, devices)
}

// SendCommand POST /api/v1/commands 下发遥控指令
// JD-岗位职责2：指令下发接口
func (h *DeviceHandler) SendCommand(c *gin.Context) {
	var req struct {
		DeviceID string `json:"device_id" binding:"required"`
		Command  string `json:"command" binding:"required"`
		Params   string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}

	cmd := mqtt.CommandPayload{
		DeviceID: req.DeviceID,
		Command:  req.Command,
		Params:   req.Params,
	}
	logEntry, err := h.deviceService.SendCommand(cmd)
	if err != nil {
		if err == service.ErrDuplicateCommand {
			Fail(c, 409, "指令重复下发，请稍后再试")
			return
		}
		Fail(c, 500, "指令下发失败: "+err.Error())
		return
	}
	Ok(c, logEntry)
}

// GetDeviceHistory GET /api/v1/history/:device_id 查询设备历史数据
// JD-岗位职责4：历史数据查询接口
func (h *DeviceHandler) GetDeviceHistory(c *gin.Context) {
	deviceID := c.Param("device_id")
	if deviceID == "" {
		Fail(c, 400, "设备ID不能为空")
		return
	}

	limit := 20
	offset := 0
	records, total, err := h.deviceService.GetDeviceHistory(deviceID, limit, offset)
	if err != nil {
		Fail(c, 500, "查询历史数据失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"total":   total,
			"records": records,
		},
	})
}
