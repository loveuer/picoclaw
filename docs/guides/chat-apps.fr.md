# 💬 Configuration des Applications de Chat

> Retour au [README](../project/README.fr.md)

## 💬 Applications de Chat

Communiquez avec votre PicoClaw via Telegram, QQ, DingTalk, WeCom, Feishu, OneBot, MQTT.


| Canal                | Difficulté         | Description                                           | Documentation                                                                                                    |
| -------------------- | ------------------ | ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| **Telegram**         | ⭐ Facile          | Recommandé, transcription vocale, long polling (pas d'IP publique requise) | [Documentation](../channels/telegram/README.fr.md)                                                  |
| **Weixin**           | ⭐ Facile          | Scan QR natif (API Tencent iLink)                     | [Documentation](#weixin)                                                                                         |
| **QQ**               | ⭐⭐ Moyen         | API bot officielle, communauté chinoise               | [Documentation](../channels/qq/README.fr.md)                                                                    |
| **DingTalk**         | ⭐⭐ Moyen         | Mode Stream (pas d'IP publique requise), entreprise   | [Documentation](../channels/dingtalk/README.fr.md)                                                              |
| **WeCom (企业微信)** | ⭐⭐⭐ Avancé      | Bot groupe (Webhook), app personnalisée (API), AI Bot | [Guide](../channels/wecom/README.fr.md) |
| **Feishu (飞书)**    | ⭐⭐⭐ Avancé      | Collaboration entreprise, fonctionnalités riches      | [Documentation](../channels/feishu/README.fr.md)                                                                |
| **OneBot**           | ⭐⭐ Moyen         | Compatible NapCat/Go-CQHTTP, écosystème communautaire | [Documentation](../channels/onebot/README.fr.md)                                                                |
| **MQTT**             | ⭐ Facile          | N'importe quel client MQTT via broker pub/sub         | [Documentation](../channels/mqtt/README.fr.md)                                                                  |

<a id="telegram"></a>
<details>
<summary><b>Telegram</b> (Recommandé)</summary>

**1. Créer un bot**

* Ouvrez Telegram, recherchez `@BotFather`
* Envoyez `/newbot`, suivez les instructions
* Copiez le token

**2. Configurer**

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

> Obtenez votre identifiant utilisateur via `@userinfobot` sur Telegram.

**3. Lancer**

```bash
picoclaw gateway
```

**4. Menu de commandes Telegram (enregistré automatiquement au démarrage)**

PicoClaw conserve les définitions de commandes dans un registre partagé unique. Au démarrage, Telegram enregistre automatiquement les commandes bot prises en charge (par exemple `/start`, `/help`, `/show`, `/list`, `/use`, `/btw`) afin que le menu de commandes et le comportement à l'exécution restent synchronisés.
L'enregistrement du menu de commandes Telegram reste une découverte UX locale au canal ; l'exécution générique des commandes est gérée de manière centralisée dans la boucle agent via l'exécuteur de commandes.

Si l'enregistrement des commandes échoue (erreurs transitoires réseau/API), le canal démarre quand même et PicoClaw réessaie l'enregistrement en arrière-plan.

Vous pouvez aussi gerer les competences installees directement depuis Telegram :

- `/list skills`
- `/use <skill> <message>`
- `/use <skill>` puis envoyer la vraie requete dans le message suivant
- `/use clear`
- `/btw <question>` pour poser une question annexe immediate sans modifier l'historique actif de la session ; `/btw` est traite comme une requete directe sans outils et n'entre pas dans le flux normal d'execution des outils

</details>

<a id="weixin"></a>
<details>
<summary><b>Weixin</b> (WeChat Personnel)</summary>

PicoClaw prend en charge la connexion à votre compte WeChat personnel via l'API officielle Tencent iLink.

**1. Connexion**

Lancez le flux de connexion interactif par QR code :
```bash
picoclaw auth weixin
```
Scannez le QR code affiché avec votre application WeChat mobile. Une fois connecté, le token est sauvegardé dans votre configuration.

**2. Configurer**

(Optionnel) Ajoutez votre identifiant utilisateur WeChat dans `allow_from` pour restreindre qui peut envoyer des messages au bot :
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

**3. Lancer**
```bash
picoclaw gateway
```

</details>

<a id="qq"></a>
<details>
<summary><b>QQ</b></summary>

**Configuration rapide (recommandée)**

QQ Open Platform propose une page de configuration en un clic pour les bots compatibles OpenClaw :

1. Ouvrez [QQ Bot Quick Start](https://q.qq.com/qqbot/openclaw/index.html) et scannez le QR code pour vous connecter
2. Un bot est créé automatiquement — copiez l'**App ID** et l'**App Secret**
3. Configurez PicoClaw :

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

4. Lancez `picoclaw gateway` et ouvrez QQ pour discuter avec votre bot

> L'App Secret n'est affiché qu'une seule fois. Enregistrez-le immédiatement — le consulter à nouveau forcera une réinitialisation.
>
> Les bots créés via la page de configuration rapide sont initialement réservés au créateur et ne prennent pas en charge les discussions de groupe. Pour activer l'accès en groupe, configurez le mode sandbox sur la [QQ Open Platform](https://q.qq.com/).

**Configuration manuelle**

Si vous préférez créer le bot manuellement :

* Connectez-vous sur [QQ Open Platform](https://q.qq.com/) pour vous inscrire en tant que développeur
* Créez un bot QQ — personnalisez son avatar et son nom
* Copiez l'**App ID** et l'**App Secret** depuis les paramètres du bot
* Configurez comme indiqué ci-dessus et lancez `picoclaw gateway`

</details>

<a id="dingtalk"></a>
<details>
<summary><b>DingTalk</b></summary>

**1. Créer un bot**

* Allez sur [Open Platform](https://open.dingtalk.com/)
* Créez une application interne
* Copiez le Client ID et le Client Secret

**2. Configurer**

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

> Définissez `allow_from` vide pour autoriser tous les utilisateurs, ou spécifiez des identifiants DingTalk pour restreindre l'accès.

**3. Lancer**

```bash
picoclaw gateway
```
</details>

<a id="wecom"></a>
<details>
<summary><b>WeCom (企业微信)</b></summary>

PicoClaw prend en charge trois types d'intégration WeCom :

**Option 1 : WeCom Bot (Bot)** - Configuration plus facile, prend en charge les discussions de groupe
**Option 2 : WeCom App (Application personnalisée)** - Plus de fonctionnalités, messagerie proactive, chat privé uniquement
**Option 3 : WeCom AI Bot (Bot IA)** - Bot IA officiel, réponses en streaming, prend en charge les discussions de groupe et privées

Voir le [Guide de Configuration WeCom](../channels/wecom/README.fr.md) pour les instructions détaillées.

**Configuration rapide - WeCom Bot :**

**1. Créer un bot**

* Allez dans la console d'administration WeCom → Discussion de groupe → Ajouter un bot de groupe
* Copiez l'URL du webhook (format : `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx`)

**2. Configurer**

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


**Configuration rapide - WeCom App :**

**1. Créer une application**

* Allez dans la console d'administration WeCom → Gestion des applications → Créer une application
* Copiez **AgentId** et **Secret**
* Allez sur la page "Mon entreprise", copiez **CorpID**

**2. Configurer la réception des messages**

* Dans les détails de l'application, cliquez sur "Recevoir les messages" → "Configurer l'API"
* Définissez l'URL à `http://your-server:18790/webhook/wecom-app`
* Générez **Token** et **EncodingAESKey**

**3. Configurer**

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

**4. Lancer**

```bash
picoclaw gateway
```

> **Note** : Les callbacks webhook WeCom sont servis sur le port Gateway (par défaut 18790). Utilisez un reverse proxy pour HTTPS.

**Configuration rapide - WeCom AI Bot :**

**1. Créer un AI Bot**

* Allez dans la console d'administration WeCom → Gestion des applications → AI Bot
* Dans les paramètres du AI Bot, configurez l'URL de callback : `http://your-server:18790/webhook/wecom-aibot`
* Copiez **Token** et cliquez sur "Générer aléatoirement" pour **EncodingAESKey**

**2. Configurer**

```json
{
  "channel_list": {
    "wecom_aibot": {
      "enabled": true,
      "token": "YOUR_TOKEN",
      "encoding_aes_key": "YOUR_43_CHAR_ENCODING_AES_KEY",
      "webhook_path": "/webhook/wecom-aibot",
      "allow_from": [],
      "welcome_message": "Hello! How can I help you?",
      "processing_message": "⏳ Processing, please wait. The results will be sent shortly."
    }
  }
}
```

**3. Lancer**

```bash
picoclaw gateway
```

> **Note** : WeCom AI Bot utilise le protocole streaming pull — pas de problème de timeout de réponse. Les tâches longues (>30 secondes) basculent automatiquement vers la livraison push via `response_url`.

</details>

<a id="feishu"></a>
<details>
<summary><b>Feishu (飞书)</b></summary>

PicoClaw se connecte à Feishu via le mode WebSocket/SDK — aucune URL webhook publique ni serveur de callback nécessaire.

**1. Créer une application**

* Allez sur [Feishu Open Platform](https://open.feishu.cn/) et créez une application
* Dans les paramètres de l'application, activez la capacité **Bot**
* Créez une version et publiez l'application (l'application doit être publiée pour prendre effet)
* Copiez l'**App ID** (commence par `cli_`) et l'**App Secret**

**2. Configurer**

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

Optionnel : `encrypt_key` et `verification_token` pour le chiffrement des événements (recommandé en production).

**3. Lancer et discuter**

```bash
picoclaw gateway
```

Ouvrez Feishu, recherchez le nom de votre bot et commencez à discuter. Vous pouvez aussi ajouter le bot à un groupe — utilisez `group_trigger.mention_only: true` pour ne répondre que lorsqu'il est @mentionné.

Pour toutes les options, voir le [Guide de Configuration du Canal Feishu](../channels/feishu/README.fr.md).

</details>

<a id="onebot"></a>
<details>
<summary><b>OneBot (QQ via protocole OneBot)</b></summary>

OneBot est un protocole ouvert pour les bots QQ. PicoClaw se connecte à toute implémentation compatible OneBot v11 (par ex. [Lagrange](https://github.com/LagrangeDev/Lagrange.Core), [NapCat](https://github.com/NapNeko/NapCatQQ)) via WebSocket.

**1. Configurer une implémentation OneBot**

Installez et exécutez un framework de bot QQ compatible OneBot v11. Activez son serveur WebSocket.

**2. Configurer**

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

| Champ | Description |
|-------|-------------|
| `ws_url` | URL WebSocket de l'implémentation OneBot |
| `access_token` | Token d'accès pour l'authentification (si configuré dans OneBot) |
| `reconnect_interval` | Intervalle de reconnexion en secondes (par défaut : 5) |

**3. Lancer**

```bash
picoclaw gateway
```

</details>

<a id="mqtt"></a>
<details>
<summary><b>MQTT</b></summary>

N'importe quel client MQTT peut communiquer avec PicoClaw via un broker. Les appareils ou services publient des requêtes vers le broker ; PicoClaw s'abonne, les traite et publie les réponses en retour.

**1. Configurer**

```json
{
  "channel_list": {
    "mqtt": {
      "enabled": true,
      "type": "mqtt",
      "settings": {
        "broker": "ssl://votre-broker:8883",
        "agent_id": "assistant",
        "topic_prefix": "/picoclaw",
        "keep_alive": 60,
        "qos": 0
      }
    }
  }
}
```

Nom d'utilisateur et mot de passe dans `~/.picoclaw/.security.yml` :

```yaml
channel_list:
  mqtt:
    settings:
      username: votre_utilisateur
      password: votre_mot_de_passe
```

**Format des topics**

```
{prefix}/{agent_id}/{client_id}/request    # Client → PicoClaw
{prefix}/{agent_id}/{client_id}/response   # PicoClaw → Client
```

Le `client_id` est défini par votre application cliente pour identifier les appareils ou sessions.

**2. Lancer**

```bash
picoclaw gateway
```

**3. Tester**

```bash
mosquitto_pub -t "/picoclaw/assistant/device1/request" \
  -m '{"text": "Bonjour"}'

mosquitto_sub -t "/picoclaw/assistant/device1/response"
```

Pour les options complètes, voir [Documentation du canal MQTT](../channels/mqtt/README.fr.md).

</details>
