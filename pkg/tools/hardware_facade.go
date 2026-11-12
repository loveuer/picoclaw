package tools

import hardwaretools "github.com/sipeed/picoclaw/pkg/tools/hardware"

type SerialTool = hardwaretools.SerialTool

func NewSerialTool() *SerialTool {
	return hardwaretools.NewSerialTool()
}
