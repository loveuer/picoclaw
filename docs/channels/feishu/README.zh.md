> 返回 [README](../../../README.md)

# 飞书

飞书（国际版名称：Lark）是字节跳动旗下的企业协作平台。本 fork 使用 WebSocket 长连接接收事件，无需公网 Webhook 回调地址。

## 配置

> 配置结构：`enabled`、`type`、`allow_from` 等公共字段位于渠道条目，Token、App ID 等专有字段位于 `settings`。`.security.yml` 使用 `channel_list.<名称>.settings` 保存渠道密钥；其中的同名值会覆盖主配置。

```json
{
  "channel_list": {
    "feishu": {
      "enabled": true,
      "type": "feishu",
      "allow_from": [],
      "settings": {
        "app_id": "cli_xxx",
        "app_secret": "xxx",
        "encrypt_key": "",
        "verification_token": "",
        "is_lark": false
      }
    }
  }
}
```

| 字段                  | 类型   | 必填 | 描述                                                                                             |
| --------------------- | ------ | ---- | ------------------------------------------------------------------------------------------------ |
| enabled               | bool   | 是   | 是否启用飞书频道                                                                                 |
| app_id                | string | 是   | 飞书应用的 App ID(以cli\_开头)                                                                   |
| app_secret            | string | 是   | 飞书应用的 App Secret                                                                            |
| encrypt_key           | string | 否   | 事件回调加密密钥                                                                                 |
| verification_token    | string | 否   | 可选事件校验 Token，长连接模式无需配置公网回调 URL                                                                       |
| allow_from            | array  | 否   | 用户ID白名单，空表示所有用户                                                                     |
| random_reaction_emoji | array  | 否   | 随机添加的表情列表，空则使用默认 "Typing"（敲键盘）                                                           |
| is_lark               | bool   | 否   | 是否使用 Lark 国际版域名（`open.larksuite.com`），默认为 `false`（使用飞书域名 `open.feishu.cn`） |

## 设置流程

1. 前往 [飞书开放平台](https://open.feishu.cn/)（国际版用户请前往 [Lark 开放平台](https://open.larksuite.com/)）创建应用程序
2. 获取 App ID 和 App Secret
3. 在开放平台的事件订阅中选择长连接接收事件，并订阅所需的消息事件
4. 设置加密(可选,生产环境建议启用)
5. 将 App ID、App Secret、Encrypt Key 和 Verification Token(如果启用加密) 填入配置文件中
6. 自定义你希望 PicoClaw react 你消息时的表情（可选, Reference URL: [Feishu Emoji List](https://open.larkoffice.com/document/server-docs/im-v1/message-reaction/emojis-introduce))

## 平台限制

> ⚠️ **飞书通道不支持 32 位设备。** 飞书官方 SDK 仅提供 64 位构建，armv6 / armv7 / mipsle 等 32 位架构无法使用飞书通道。如需在 32 位设备上接入即时通讯，请改用 Telegram 或 OneBot 等通道。
