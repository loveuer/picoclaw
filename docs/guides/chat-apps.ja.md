# 💬 チャットアプリ設定

> [README](../project/README.ja.md) に戻る

## 💬 チャットアプリ連携

PicoClaw は複数のチャットプラットフォームをサポートしており、Agent をどこにでも接続できます。


### チャネル一覧

| チャネル             | セットアップ難易度 | 特徴                                      | ドキュメント                                                                                                    |
| -------------------- | ------------------ | ----------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| **Telegram**         | ⭐ 簡単            | 推奨、音声テキスト変換対応、ロングポーリング（公開 IP 不要） | [ドキュメント](../channels/telegram/README.ja.md)                                                             |
| **微信 (Weixin)**    | ⭐ 簡単            | ネイティブ QR スキャン（Tencent iLink API）| [ドキュメント](#weixin)                                                               |
| **QQ**               | ⭐⭐ 中程度        | 公式ボット API、中国コミュニティ向け       | [ドキュメント](../channels/qq/README.ja.md)                                                                   |
| **DingTalk**         | ⭐⭐ 中程度        | Stream モード（公開 IP 不要）、企業向け    | [ドキュメント](../channels/dingtalk/README.ja.md)                                                             |
| **WeCom (企業微信)** | ⭐⭐⭐ やや難      | グループ Bot (Webhook)、カスタムアプリ (API)、AI Bot 対応 | [ガイド](../channels/wecom/README.ja.md) |
| **Feishu (飛書)**    | ⭐⭐⭐ やや難      | エンタープライズコラボレーション、機能豊富 | [ドキュメント](../channels/feishu/README.ja.md)                                                               |
| **OneBot**           | ⭐⭐ 中程度        | NapCat/Go-CQHTTP 互換、コミュニティエコシステム充実 | [ドキュメント](../channels/onebot/README.ja.md)                                                               |
| **MQTT**             | ⭐ 簡単            | ブローカー経由で任意の MQTT クライアントと通信 | [ドキュメント](../channels/mqtt/README.ja.md)                                                                 |

---

<a id="telegram"></a>
<details>
<summary><b>Telegram</b>（推奨）</summary>

**1. Bot を作成**

* Telegram を開き、`@BotFather` を検索
* `/newbot` を送信し、プロンプトに従う
* Token をコピー

**2. 設定**

```json
{
  "channel_list": {
    "telegram": {
      "enabled": true,
      "type": "telegram",
      "token": "YOUR_BOT_TOKEN",
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

> Telegram の `@userinfobot` から User ID を取得できます。

**3. 実行**

```bash
picoclaw gateway
```

**4. Telegram コマンドメニュー（起動時に自動登録）**

PicoClaw は統一されたコマンド定義を使用します。起動時に Telegram がサポートするコマンド（例: `/start`、`/help`、`/show`、`/list`、`/use`、`/btw`）を Bot コマンドメニューに自動登録し、メニュー表示と実際の動作を一致させます。
Telegram 側はコマンドメニュー登録機能を保持し、汎用コマンドの実行は Agent Loop 内の commands executor で統一的に処理されます。

ネットワークや API の一時的なエラーで登録に失敗しても、チャネルの起動はブロックされません。システムがバックグラウンドで自動リトライします。

</details>

<a id="weixin"></a>
<details>
<summary><b>微信 (Weixin)</b></summary>

PicoClaw は Tencent iLink 公式 API を使用して WeChat 個人アカウントへの接続をサポートしています。

**1. ログイン**

インタラクティブな QR ログインフローを実行します：
```bash
picoclaw auth weixin
```
WeChat モバイルアプリで表示された QR コードをスキャンしてください。ログイン成功後、トークンが設定ファイルに保存されます。

**2. 設定**

（オプション）ボットと会話できるユーザーを制限するために `allow_from` に WeChat ユーザー ID を追加します：
```json
{
  "channel_list": {
    "weixin": {
      "enabled": true,
      "type": "weixin",
      "token": "YOUR_TOKEN",
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

**3. 実行**
```bash
picoclaw gateway
```

</details>

<a id="qq"></a>
<details>
<summary><b>QQ</b></summary>

**クイックセットアップ（推奨）**

QQ 開放プラットフォームでは、OpenClaw 互換ボットのワンクリックセットアップページが提供されています：

1. [QQ Bot クイックスタート](https://q.qq.com/qqbot/openclaw/index.html) を開き、QR コードをスキャンしてログイン
2. ボットが自動的に作成されます — **App ID** と **App Secret** をコピー
3. PicoClaw を設定：

```json
{
  "channel_list": {
    "qq": {
      "enabled": true,
      "type": "qq",
      "app_id": "YOUR_APP_ID",
      "app_secret": "YOUR_APP_SECRET",
      "allow_from": []
    }
  }
}
```

4. `picoclaw gateway` を実行し、QQ を開いてボットとチャット

> App Secret は一度しか表示されません。すぐに保存してください — 再度表示するとリセットされます。
>
> クイックセットアップで作成されたボットは、最初は作成者のみが使用でき、グループチャットには対応していません。グループアクセスを有効にするには、[QQ 開放プラットフォーム](https://q.qq.com/) でサンドボックスモードを設定してください。

**手動セットアップ**

ボットを手動で作成する場合：

* [QQ 開放プラットフォーム](https://q.qq.com/) にログインして開発者登録
* QQ ボットを作成 — アバターと名前をカスタマイズ
* ボット設定から **App ID** と **App Secret** をコピー
* 上記の設定を行い、`picoclaw gateway` を実行

</details>

<a id="dingtalk"></a>
<details>
<summary><b>DingTalk</b></summary>

**1. Bot を作成**

* [開放プラットフォーム](https://open.dingtalk.com/) にアクセス
* 内部アプリを作成
* Client ID と Client Secret をコピー

**2. 設定**

```json
{
  "channel_list": {
    "dingtalk": {
      "enabled": true,
      "type": "dingtalk",
      "client_id": "YOUR_CLIENT_ID",
      "client_secret": "YOUR_CLIENT_SECRET",
      "allow_from": []
    }
  }
}
```

> `allow_from` を空にするとすべてのユーザーを許可します。DingTalk ユーザー ID を指定してアクセスを制限することもできます。

**3. 実行**

```bash
picoclaw gateway
```

</details>

<a id="feishu"></a>
<details>
<summary><b>Feishu (飛書)</b></summary>

PicoClaw は WebSocket/SDK モードで飛書に接続します — 公開 Webhook URL やコールバックサーバーは不要です。

**1. アプリを作成**

* [飛書開放プラットフォーム](https://open.feishu.cn/) にアクセスしてアプリケーションを作成
* アプリ設定で **ボット** 機能を有効化
* バージョンを作成してアプリを公開（アプリは公開しないと有効になりません）
* **App ID**（`cli_` で始まる）と **App Secret** をコピー

**2. 設定**

```json
{
  "channel_list": {
    "feishu": {
      "enabled": true,
      "type": "feishu",
      "app_id": "cli_xxx",
      "app_secret": "YOUR_APP_SECRET",
      "allow_from": []
    }
  }
}
```

オプション：`encrypt_key` と `verification_token` でイベント暗号化（本番環境推奨）。

**3. 実行してチャット**

```bash
picoclaw gateway
```

飛書を開き、ボット名を検索してチャットを開始できます。ボットをグループに追加することもできます — `group_trigger.mention_only: true` を設定すると @メンション時のみ応答します。

詳細なオプションについては [飛書チャネル設定ガイド](../channels/feishu/README.ja.md) を参照してください。

</details>

<a id="wecom"></a>
<details>
<summary><b>WeCom (企業微信)</b></summary>

PicoClaw は 3 種類の WeCom 統合をサポートしています：

**方式 1: グループ Bot (Bot)** — セットアップ簡単、グループチャット対応
**方式 2: カスタムアプリ (App)** — より多機能、プロアクティブメッセージング、プライベートチャットのみ
**方式 3: AI Bot** — 公式 AI Bot、ストリーミング返信、グループ・プライベートチャット対応

詳細なセットアップ手順は [WeCom 設定ガイド](../channels/wecom/README.ja.md) を参照してください。

**クイックセットアップ — グループ Bot：**

**1. Bot を作成**

* WeCom 管理コンソール → グループチャット → グループ Bot を追加
* Webhook URL をコピー（形式：`https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx`）

**2. 設定**

```json
{
  "channel_list": {
    "wecom": {
      "enabled": true,
      "type": "wecom",
      "token": "YOUR_TOKEN",
      "encoding_aes_key": "YOUR_ENCODING_AES_KEY",
      "webhook_url": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=YOUR_KEY",
      "webhook_path": "/webhook/wecom",
      "allow_from": []
    }
  }
}
```


**クイックセットアップ — カスタムアプリ：**

**1. アプリを作成**

* WeCom 管理コンソール → アプリ管理 → アプリを作成
* **AgentId** と **Secret** をコピー
* 「マイ企業」ページで **CorpID** をコピー

**2. メッセージ受信を設定**

* アプリ詳細で「メッセージ受信」→「API を設定」をクリック
* URL を `http://your-server:18790/webhook/wecom-app` に設定
* **Token** と **EncodingAESKey** を生成

**3. 設定**

```json
{
  "channel_list": {
    "wecom_app": {
      "enabled": true,
      "corp_id": "wwxxxxxxxxxxxxxxxx",
      "corp_secret": "YOUR_CORP_SECRET",
      "agent_id": 1000002,
      "token": "YOUR_TOKEN",
      "encoding_aes_key": "YOUR_ENCODING_AES_KEY",
      "webhook_path": "/webhook/wecom-app",
      "allow_from": []
    }
  }
}
```

**4. 実行**

```bash
picoclaw gateway
```

> **注意**: WeCom Webhook コールバックは Gateway ポート（デフォルト 18790）で提供されます。HTTPS にはリバースプロキシを使用してください。

**クイックセットアップ — AI Bot：**

**1. AI Bot を作成**

* WeCom 管理コンソール → アプリ管理 → AI Bot
* AI Bot 設定でコールバック URL を設定：`http://your-server:18790/webhook/wecom-aibot`
* **Token** をコピーし、「ランダム生成」をクリックして **EncodingAESKey** を取得

**2. 設定**

```json
{
  "channel_list": {
    "wecom_aibot": {
      "enabled": true,
      "token": "YOUR_TOKEN",
      "encoding_aes_key": "YOUR_43_CHAR_ENCODING_AES_KEY",
      "webhook_path": "/webhook/wecom-aibot",
      "allow_from": [],
      "welcome_message": "こんにちは！何かお手伝いできますか？",
      "processing_message": "⏳ Processing, please wait. The results will be sent shortly."
    }
  }
}
```

**3. 実行**

```bash
picoclaw gateway
```

> **注意**: WeCom AI Bot はストリーミングプルプロトコルを使用しており、返信タイムアウトの心配はありません。長時間タスク（30 秒超）は自動的に `response_url` プッシュ配信に切り替わります。

</details>

<a id="onebot"></a>
<details>
<summary><b>OneBot（OneBot プロトコル経由の QQ）</b></summary>

OneBot は QQ ボット向けのオープンプロトコルです。PicoClaw は OneBot v11 互換の実装（例：[Lagrange](https://github.com/LagrangeDev/Lagrange.Core)、[NapCat](https://github.com/NapNeko/NapCatQQ)）に WebSocket で接続します。

**1. OneBot 実装をセットアップ**

OneBot v11 互換の QQ ボットフレームワークをインストールして実行します。WebSocket サーバーを有効にしてください。

**2. 設定**

```json
{
  "channel_list": {
    "onebot": {
      "enabled": true,
      "type": "onebot",
      "ws_url": "ws://127.0.0.1:8080",
      "access_token": "",
      "allow_from": []
    }
  }
}
```

| フィールド | 説明 |
|-------|-------------|
| `ws_url` | OneBot 実装の WebSocket URL |
| `access_token` | 認証用アクセストークン（OneBot 側で設定している場合） |
| `reconnect_interval` | 再接続間隔（秒）（デフォルト：5） |

**3. 実行**

```bash
picoclaw gateway
```

</details>

<a id="mqtt"></a>
<details>
<summary><b>MQTT</b></summary>

任意の MQTT クライアントがブローカーを介して PicoClaw と通信できます。デバイスやサービスがブローカーにリクエストをパブリッシュし、PicoClaw がサブスクライブして処理し、レスポンスをパブリッシュして返します。

**1. 設定**

```json
{
  "channel_list": {
    "mqtt": {
      "enabled": true,
      "type": "mqtt",
      "settings": {
        "broker": "ssl://your-broker:8883",
        "agent_id": "assistant",
        "topic_prefix": "/picoclaw",
        "keep_alive": 60,
        "qos": 0
      }
    }
  }
}
```

ユーザー名とパスワードは `~/.picoclaw/.security.yml` に記載します：

```yaml
channel_list:
  mqtt:
    settings:
      username: your_username
      password: your_password
```

**トピック形式**

```
{prefix}/{agent_id}/{client_id}/request    # クライアント → PicoClaw
{prefix}/{agent_id}/{client_id}/response   # PicoClaw → クライアント
```

`client_id` はクライアントアプリケーションがデバイスやセッションを識別するために設定します。

**2. 起動**

```bash
picoclaw gateway
```

**3. テスト**

```bash
mosquitto_pub -t "/picoclaw/assistant/device1/request" \
  -m '{"text": "こんにちは"}'

mosquitto_sub -t "/picoclaw/assistant/device1/response"
```

完全な設定オプションは [MQTT チャンネルドキュメント](../channels/mqtt/README.ja.md) を参照してください。

</details>
