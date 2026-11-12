# 💬 Konfigurasi Aplikasi Sembang

> Kembali ke [README](../project/README.ms.md)

## 💬 Aplikasi Sembang

Berbual dengan picoclaw anda melalui Telegram, QQ, DingTalk, WeCom, Feishu, OneBot, MQTT


| Saluran          | Penyediaan                                 |
| ---------------- | ------------------------------------------ |
| **Telegram**     | Mudah (hanya token)                        |
| **QQ**           | Mudah (AppID + AppSecret)                  |
| **DingTalk**     | Sederhana (kelayakan aplikasi)             |
| **WeCom AI Bot** | Sederhana (Token + kunci AES)              |
| **Feishu**       | Sederhana (App ID + Secret, mod WebSocket) |
| **OneBot**       | Sederhana (QQ melalui protokol OneBot)     |
| **MQTT**         | Mudah (broker + agent_id)                  |

<details>
<summary><b>Telegram</b> (Disyorkan)</summary>

**1. Cipta bot**

* Buka Telegram, cari `@BotFather`
* Hantar `/newbot`, ikut arahan
* Salin token

**2. Konfigurasi**

```json
{
  "channel_list": {
    "telegram": {
      "enabled": true,
      "type": "telegram",
      "token": "YOUR_BOT_TOKEN",
      "allow_from": ["YOUR_USER_ID"],
      "use_markdown_v2": false,
    }
  }
}
```

> Dapatkan user ID anda daripada `@userinfobot` di Telegram.

**3. Jalankan**

```bash
picoclaw gateway
```

**4. Menu arahan Telegram (auto-register semasa startup)**

PicoClaw kini menyimpan definisi arahan dalam satu registry bersama. Semasa startup, Telegram akan mendaftarkan arahan bot yang disokong secara automatik (contohnya `/start`, `/help`, `/show`, `/list`, `/use`, `/btw`) supaya menu arahan dan tingkah laku runtime sentiasa selari.
Pendaftaran menu arahan Telegram kekal sebagai UX penemuan setempat saluran; pelaksanaan arahan generik dikendalikan secara berpusat dalam gelung agen melalui commands executor.

Jika pendaftaran arahan gagal (ralat sementara rangkaian/API), saluran tetap akan bermula dan PicoClaw akan mencuba semula pendaftaran di latar belakang.

Anda juga boleh mengurus skill yang dipasang terus dari Telegram:

- `/list skills`
- `/use <skill> <message>`
- `/use <skill>` kemudian hantar permintaan sebenar dalam mesej seterusnya
- `/use clear`
- `/btw <question>` untuk bertanya soalan sampingan segera tanpa mengubah sejarah sesi aktif; `/btw` dikendalikan sebagai pertanyaan langsung tanpa tool dan tidak memasuki aliran pelaksanaan tool biasa

**4. Pemformatan Lanjutan**
Anda boleh menetapkan `use_markdown_v2: true` untuk mengaktifkan pilihan pemformatan yang lebih maju. Ini membolehkan bot menggunakan keseluruhan set ciri Telegram MarkdownV2, termasuk gaya bersarang, spoiler, dan blok lebar tetap tersuai.

</details>

<details>
<summary><b>QQ</b></summary>

**1. Cipta bot**

- Pergi ke [QQ Open Platform](https://q.qq.com/#)
- Cipta aplikasi → Dapatkan **AppID** dan **AppSecret**

**2. Konfigurasi**

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

> Tetapkan `allow_from` kepada kosong untuk membenarkan semua pengguna, atau nyatakan nombor QQ untuk mengehadkan akses.

**3. Jalankan**

```bash
picoclaw gateway
```

</details>

<details>
<summary><b>DingTalk</b></summary>

**1. Cipta bot**

* Pergi ke [Open Platform](https://open.dingtalk.com/)
* Cipta aplikasi dalaman
* Salin Client ID dan Client Secret

**2. Konfigurasi**

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

> Tetapkan `allow_from` kepada kosong untuk membenarkan semua pengguna, atau nyatakan user ID DingTalk untuk mengehadkan akses.

**3. Jalankan**

```bash
picoclaw gateway
```
</details>

<details>
<summary><b>WeCom (企业微信)</b></summary>

PicoClaw menyokong tiga jenis integrasi WeCom:

**Pilihan 1: WeCom Bot (Bot)** - Penyediaan lebih mudah, menyokong sembang kumpulan
**Pilihan 2: WeCom App (Custom App)** - Lebih banyak ciri, pemesejan proaktif, sembang peribadi sahaja
**Pilihan 3: WeCom AI Bot (AI Bot)** - AI Bot rasmi, balasan streaming, menyokong sembang kumpulan & peribadi

Lihat [Panduan Konfigurasi WeCom](../channels/wecom/README.zh.md) untuk arahan penyediaan terperinci.

**Quick Setup - WeCom Bot:**

**1. Cipta bot**

* Pergi ke WeCom Admin Console → Group Chat → Add Group Bot
* Salin webhook URL (format: `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx`)

**2. Konfigurasi**

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


**Quick Setup - WeCom App:**

**1. Cipta aplikasi**

* Pergi ke WeCom Admin Console → App Management → Create App
* Salin **AgentId** dan **Secret**
* Pergi ke halaman "My Company", salin **CorpID**

**2. Konfigurasi penerimaan mesej**

* Dalam butiran aplikasi, klik "Receive Message" → "Set API"
* Tetapkan URL kepada `http://your-server:18790/webhook/wecom-app`
* Jana **Token** dan **EncodingAESKey**

**3. Konfigurasi**

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

**4. Jalankan**

```bash
picoclaw gateway
```

> **Nota**: Callback webhook WeCom diservis pada port Gateway (lalai 18790). Gunakan reverse proxy untuk HTTPS.

**Quick Setup - WeCom AI Bot:**

**1. Cipta AI Bot**

* Pergi ke WeCom Admin Console → App Management → AI Bot
* Dalam tetapan AI Bot, konfigurasikan callback URL: `http://your-server:18791/webhook/wecom-aibot`
* Salin **Token** dan klik "Random Generate" untuk **EncodingAESKey**

**2. Konfigurasi**

```json
{
  "channel_list": {
    "wecom_aibot": {
      "enabled": true,
      "token": "YOUR_TOKEN",
      "encoding_aes_key": "YOUR_43_CHAR_ENCODING_AES_KEY",
      "webhook_path": "/webhook/wecom-aibot",
      "allow_from": [],
      "welcome_message": "Hello! How can I help you?"
    }
  }
}
```

**3. Jalankan**

```bash
picoclaw gateway
```

> **Nota**: WeCom AI Bot menggunakan protokol streaming pull — tiada isu timeout balasan. Tugasan panjang (>30 saat) akan bertukar secara automatik kepada penghantaran push `response_url`.

</details>

<a id="mqtt"></a>
<details>
<summary><b>MQTT</b></summary>

Mana-mana client MQTT boleh berkomunikasi dengan PicoClaw melalui broker. Peranti atau perkhidmatan menerbitkan permintaan ke broker; PicoClaw melanggan, memproses dan menerbitkan respons kembali.

**1. Konfigurasi**

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

Nama pengguna dan kata laluan dalam `~/.picoclaw/.security.yml`:

```yaml
channel_list:
  mqtt:
    settings:
      username: nama_pengguna
      password: kata_laluan
```

**Format topik**

```
{prefix}/{agent_id}/{client_id}/request    # Client → PicoClaw
{prefix}/{agent_id}/{client_id}/response   # PicoClaw → Client
```

`client_id` ditetapkan oleh aplikasi client anda untuk mengenal pasti peranti atau sesi.

**2. Jalankan**

```bash
picoclaw gateway
```

**3. Uji**

```bash
mosquitto_pub -t "/picoclaw/assistant/device1/request" \
  -m '{"text": "Helo"}'

mosquitto_sub -t "/picoclaw/assistant/device1/response"
```

Untuk semua pilihan konfigurasi, lihat [Dokumentasi Saluran MQTT](../channels/mqtt/README.md).

</details>
