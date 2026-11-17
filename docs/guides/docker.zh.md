# Docker 本地构建与运行

> 返回 [README](../../README.md)

本 fork 没有发布 Docker 镜像。必须从本仓库构建，避免拉取上游镜像后运行已移除的功能或不同的配置格式。Compose 中的 `docker.io/sipeed/picoclaw:latest` 是历史遗留的本地镜像标签，不代表本 fork 的发布源。

## 初始化

需要 Git、Docker 和 Compose 插件。从仓库根目录运行：

```bash
git clone https://github.com/loveuer/picoclaw.git
cd picoclaw
docker compose -f docker/docker-compose.yml --profile gateway build

# 初始化 docker/data/ 中的配置和工作区，不启动 Gateway
# 已有配置时请按 onboard 的提示处理
docker compose -f docker/docker-compose.yml run --rm \
  --entrypoint picoclaw picoclaw-agent onboard
```

编辑 `docker/data/config.json`，按照[模型配置](providers.zh.md)设置 OpenAI 或 Anthropic 模型，并启用需要的渠道。密钥也可能保存在 `docker/data/.security.yml`，该文件中的同名值会覆盖主配置。

当前格式是 `channel_list.<名称>.settings`，参见 [完整配置示例](../../config/config.example.json)。

## 运行

```bash
# 只使用刚才在本地构建的镜像
docker compose -f docker/docker-compose.yml --profile gateway up -d --no-build --pull never

docker compose -f docker/docker-compose.yml logs -f picoclaw-gateway
docker compose -f docker/docker-compose.yml --profile gateway down

# 单次提问或交互式 CLI
docker compose -f docker/docker-compose.yml run --rm --pull never picoclaw-agent -m "2+2 等于几？"
docker compose -f docker/docker-compose.yml run --rm --pull never picoclaw-agent
```

数据目录 `docker/data/` 挂载到容器内的 `/root/.picoclaw`，停止容器不会删除配置和工作区。Gateway 默认监听容器内的本地地址，Compose 未发布端口；如需从宿主机访问 `/health`、`/ready`，需同时设置 `gateway.host: "0.0.0.0"` 和所需的端口映射。没有 WebUI 或通用 REST 聊天接口。

## 更新

更新本地源码后重新构建并启动，保留 `docker/data/`：

```bash
git pull --ff-only
docker compose -f docker/docker-compose.yml --profile gateway build
docker compose -f docker/docker-compose.yml --profile gateway up -d --no-build --pull never
```

不要用 `docker compose pull` 更新本 fork，它会拉取历史标签对应的上游镜像。已有工作区的内置 Skill 不会随镜像更新自动覆盖，参见[办公 Skills](office-skills.zh.md)。当前普通 Gateway 镜像没有为办公 Skill 配置 Docker daemon 访问；需要办公能力时，优先按主 README 在宿主机部署并单独准备办公环境。

本地仍保留 full/heavy 镜像构建变体，是否收敛这些变体是后续维护事项；它们不属于自动发布产物。
