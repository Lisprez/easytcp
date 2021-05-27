package easytcp

import (
	"github.com/DarthPestilane/easytcp/logger"
	"github.com/DarthPestilane/easytcp/server"
	"github.com/sirupsen/logrus"
)

// SetLogger 设置日志对象
func SetLogger(log *logrus.Logger) {
	logger.Default = log
}

// NewTcp 创建 tcp server
func NewTcp(opt server.TcpOption) *server.TcpServer {
	return server.NewTcp(opt)
}
