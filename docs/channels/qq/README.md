> Back to [README](../../../README.md)

# QQ

PicoClaw provides QQ support via the official Bot API from the QQ Open Platform.

## Configuration

> Configuration: common fields such as `enabled`, `type`, and `allow_from` belong on the channel entry; channel-specific fields belong under `settings`. Credentials saved under `channel_list.<name>.settings` in `.security.yml` override the main JSON configuration.

```json
{
  "channel_list": {
    "qq": {
      "enabled": true,
      "type": "qq",
      "allow_from": [],
      "settings": {
        "app_id": "YOUR_APP_ID",
        "app_secret": "YOUR_APP_SECRET"
      }
    }
  }
}
```

| Field      | Type   | Required | Description                                              |
| ---------- | ------ | -------- | -------------------------------------------------------- |
| enabled    | bool   | Yes      | Whether to enable the QQ channel                         |
| app_id     | string | Yes      | App ID of the QQ bot application                         |
| app_secret | string | Yes      | App Secret of the QQ bot application                     |
| allow_from | array  | No       | Allowlist of user IDs; empty means all users are allowed |

## Setup

### Quick Setup (Recommended)

The QQ Open Platform provides a one-click creation entry:

1. Open [QQ Bot Quick Create](https://q.qq.com/qqbot/openclaw/index.html) and log in by scanning the QR code
2. The system automatically creates a bot — copy the **App ID** and **App Secret**
3. Fill in the credentials in the PicoClaw configuration file
4. Run `picoclaw gateway` to start the service
5. Open QQ and start chatting with the bot

> The App Secret is only shown once — save it immediately. Viewing it again will force a reset.
>
> Bots created via the quick entry are for the creator's personal use only and do not support group chats. For group chat support, configure sandbox mode on the [QQ Open Platform](https://q.qq.com/).

### Manual Setup

1. Log in to the [QQ Open Platform](https://q.qq.com/) with your QQ account and register as a developer
2. Create a QQ bot and customize its avatar and name
3. Obtain the **App ID** and **App Secret** from the bot settings
4. Fill in the credentials in the PicoClaw configuration file
5. Run `picoclaw gateway` to start the service
6. Search for your bot in QQ and start chatting

> During development, it is recommended to enable sandbox mode and add test users and groups to the sandbox for debugging.
