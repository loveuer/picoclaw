# 办公文件 Skills

`docx`、`xlsx`、`pptx` 基于 Anthropic 的文档 Skills 适配，来源版本及 Apache-2.0 许可保留在各 Skill 目录。它们提供文件处理流程和辅助脚本，Docker 镜像提供实际执行工具。

## 安装与更新

在运行 PicoClaw 的用户下执行。以下以默认工作区为例，自定义工作区请替换路径：

```bash
picoclaw skills install-builtin docx xlsx pptx
bash ~/.picoclaw/workspace/tools/office/office.sh setup
bash ~/.picoclaw/workspace/tools/office/office.sh check
```

新工作区初始化时已带有这些资源，仍需要手动执行一次 `setup`。这不会启动常驻容器或重启 Gateway。Linux 是本次验证的环境；使用者需要已有可用的 Docker，脚本不自动安装 Docker、不执行 sudo、不修改服务权限。

`install-builtin` 不带名称时安装所有内置 Skill，默认跳过已有目录，包括共享办公环境。`--force` 替换所选目录和共享环境，慎用于有本地修改的目录。不会清理其他自定义 Skill。内置资源来自当前二进制，升级资源前先升级二进制。

`setup` 根据 Dockerfile、Python/npm 锁定文件和容器入口生成镜像标识，资源变化后重新构建。Python/npm 依赖固定版本；基础镜像和 Debian 系统包仍随构建时的软件源变化。

## 执行方式

```bash
# 检查所有依赖、字体及镜像
bash ~/.picoclaw/workspace/tools/office/office.sh check

# 读取 Word；请用真实绝对路径替换示例
bash ~/.picoclaw/workspace/tools/office/office.sh run -- \
  pandoc -t markdown /home/your-user/.picoclaw/workspace/report.docx

# 执行工作区内的 Python 或 Node.js 生成脚本
bash ~/.picoclaw/workspace/tools/office/office.sh run -- \
  python /home/your-user/.picoclaw/workspace/create_excel.py
```

参数逐个传递，不做额外的 shell 求值。需要多条命令时使用 `run -- bash -c '...'`，正确引用文件路径。Skill 中的 `scripts/...` 指该 Skill 自己的辅助脚本目录，应传入真实绝对路径。调用者在工作区内时保留工作目录，否则使用工作区根目录。

每次运行创建临时容器，结束后删除；镜像保留。文件以当前用户的 UID/GID 写回工作区，不会变成 root 所有。容器没有网络，只挂载工作区，不挂载配置密钥或 Docker socket。外部文件或网络素材先通过常规工具放入工作区。文件路径不要包含逗号（Docker bind mount 参数限制）。

## 能力与限制

| Skill | 主要工具 | 注意事项 |
| --- | --- | --- |
| `docx` | docx-js、Pandoc、OOXML 辅助脚本 | 保留原文件，编辑后验证；复杂排版需要预览 |
| `xlsx` | openpyxl、pandas、LibreOffice | 写公式后重算，并读取缓存值核对；外部链接、宏和高级函数需单独验证 |
| `pptx` | PptxGenJS、OOXML 辅助脚本、缩略图工具 | 使用模板时检查布局；修改后渲染核对溢出与缺失内容 |

提供 `Noto Sans CJK SC` / `Noto Serif CJK SC` 中文字体。LibreOffice 与 Microsoft Office 的排版和公式支持可能不同，转换成功不代表视觉或业务计算结果完全正确。无法查看图片的模型应说明未完成视觉检查。

旧 `.doc`、`.ppt` 先用 LibreOffice 转为 `.docx`、`.pptx`。本次不处理飞书在线文档 API，也不改变现有渠道对文件上传、下载和发送的支持范围。

## 验证与排错

运行可重复的文件验证（输出目录必须不存在）：

```bash
bash ~/.picoclaw/workspace/tools/office/office.sh run -- \
  python /home/your-user/.picoclaw/workspace/tools/office/smoke_test.py \
  /home/your-user/.picoclaw/workspace/office-check-001
```

验证创建和修改中文 Word、Excel、PowerPoint 文件，检查批注、公式重算结果、演示页数、PDF 文本、预览图片及输出归属。不会调用模型或消耗模型 API 额度。

- 缺少 Docker 或没有 daemon 权限：由运维准备 Docker，再以 Gateway 用户运行 `check`。
- 缺少镜像：执行 `setup`；脚本不会在普通文档任务中自动构建。
- 依赖或字体检查失败：确认 Skill 与共享环境来自同一个版本，重新执行 `setup`。
- 执行策略拒绝命令：检查现有工具策略，不要为办公功能全局关闭命令限制。
- 较大的文档转换超时：按实际任务配置执行时限，保留原文件后重试。
