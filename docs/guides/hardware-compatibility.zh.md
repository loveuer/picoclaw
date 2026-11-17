# 平台与硬件功能边界

本 fork 的自动构建与发布目标为 **Linux amd64**，源码构建、发布包和自更新方式见 [README](../../README.md)。

```bash
make build GOOS=linux GOARCH=amd64
# build/picoclaw-linux-amd64
```

Makefile 仍保留其他交叉编译命令，但不代表当前验证过这些平台或设备。部署前需要自行验证目标平台、渠道 SDK 及运行行为；飞书 SDK 不支持 32 位平台。不再维护上游的芯片/开发板验证列表，也不要混用上游发行包更新本 fork。

内置 I²C、SPI、串口工具、USB 设备监控和 hardware Skill 已移除。硬件接入通过按需安装的 Skill 和外部工具完成；这不影响 CLI/Gateway 在支持的主机上运行。

办公文件处理使用单独的 Docker 环境，见[办公 Skills](office-skills.zh.md)。
