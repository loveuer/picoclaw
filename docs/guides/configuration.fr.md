# ⚙️ Guide de Configuration

> Fork configuration: `PICOCLAW_HOME` also changes the default config path; `PICOCLAW_CONFIG` overrides that path. For current model protocols and channel settings, use the [README](../../README.md) and [configuration example](../../config/config.example.json).

> Retour au [README](../../README.md)

## ⚙️ Configuration

Fichier de configuration : `~/.picoclaw/config.json`

### Variables d'Environnement

Vous pouvez remplacer les chemins par défaut à l'aide de variables d'environnement. Ceci est utile pour les installations portables, les déploiements conteneurisés ou l'exécution de PicoClaw en tant que service système. Ces variables sont indépendantes et contrôlent des chemins différents.

| Variable          | Description                                                                                                                             | Chemin par défaut         |
|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------|---------------------------|
| `PICOCLAW_CONFIG` | Remplace le chemin vers le fichier de configuration. Indique directement à PicoClaw quel `config.json` charger, en ignorant tous les autres emplacements. | `~/.picoclaw/config.json` |
| `PICOCLAW_HOME`   | Remplace le répertoire racine des données PicoClaw. Change l'emplacement par défaut du `workspace` et des autres répertoires de données. | `~/.picoclaw`             |

**Exemples :**

```bash
# Run picoclaw using a specific config file
# The workspace path will be read from within that config file
PICOCLAW_CONFIG=/etc/picoclaw/production.json picoclaw gateway

# Run picoclaw with all its data stored in /opt/picoclaw
# Config: /opt/picoclaw/config.json (PICOCLAW_HOME=/opt/picoclaw)
# Workspace will be created at /opt/picoclaw/workspace
PICOCLAW_HOME=/opt/picoclaw picoclaw agent

# Use both for a fully customized setup
PICOCLAW_HOME=/srv/picoclaw PICOCLAW_CONFIG=/srv/picoclaw/main.json picoclaw gateway
```

### Niveau de Log du Gateway

`gateway.log_level` contrôle la verbosité des logs du Gateway, configurable dans `config.json` :

```json
{
  "gateway": {
    "log_level": "warn"
  }
}
```

La valeur par défaut est `warn`. Valeurs supportées : `debug`, `info`, `warn`, `error`, `fatal`.

Peut également être surchargé via la variable d'environnement : `PICOCLAW_LOG_LEVEL=info`

### Structure du Workspace

PicoClaw stocke les données dans votre workspace configuré (par défaut : `~/.picoclaw/workspace`) :

```
~/.picoclaw/workspace/
├── sessions/          # Sessions de conversation et historique
├── memory/           # Mémoire à long terme (MEMORY.md)
├── state/            # État persistant (dernier canal, etc.)
├── cron/             # Base de données des tâches planifiées
├── skills/           # Compétences personnalisées
├── AGENT.md          # Guide de comportement de l'agent
├── HEARTBEAT.md      # Invites de tâches périodiques (vérifiées toutes les 30 min)
├── SOUL.md           # Âme de l'agent
└── USER.md           # Préférences utilisateur
```

> **Remarque :** Les modifications apportées à `AGENT.md`, `SOUL.md`, `USER.md` et `memory/MEMORY.md` sont détectées automatiquement au moment de l'exécution via le suivi de la date de modification (mtime). Il n'est **pas nécessaire de redémarrer le gateway** après avoir modifié ces fichiers — l'agent charge le nouveau contenu à la prochaine requête.

### Politique de contexte de requête

`turn_profile` est une politique facultative sous `agents.defaults.turn_profile` pour contrôler le contexte chargé par chaque nouveau tour : historique, prompt système, prompts de skills et outils autorisés. Sans cette configuration, ou avec `"enabled": false`, PicoClaw garde son comportement normal. Avec `"enabled": true`, la politique ci-dessous s'applique à chaque nouveau tour.

Tous les blocs utilisent les mêmes valeurs de `mode` :

| Mode | Signification |
| --- | --- |
| `default` | Garde le comportement normal de PicoClaw. Un bloc absent ou sans `mode` vaut `default`. |
| `off` | Désactive ce bloc pour le tour. |
| `custom` | Utilise une liste d'autorisation. Dans cette version, `custom` est pris en charge seulement pour `skills` et `tools`; l'utiliser pour `history` ou `system_prompt` produit une erreur de validation. |

Blocs disponibles :

| Bloc | Ce qu'il contrôle |
| --- | --- |
| `history` | Lecture de l'historique et du résumé, écriture des messages utilisateur/assistant/outil, ingestion de contexte, compression et résumé. |
| `system_prompt` | Injection de l'identité PicoClaw, des instructions de l'espace de travail, de la mémoire, du contexte d'exécution et du résumé. Les prompts système externes restent autorisés quand ce bloc est `off`. |
| `skills` | Chargement du catalogue de skills et du contenu des skills actifs. `custom.allow` ne garde que les noms listés. |
| `tools` | Outils exposés au modèle et autorisés à l'exécution. `custom.allow` ne garde que les outils enregistrés et listés. |

Quand `system_prompt.mode` vaut `off`, que des outils restent visibles et qu'aucun prompt système externe n'est fourni, PicoClaw réutilise sa règle existante d'utilisation des outils comme prompt minimal de secours. Si `tools.mode` vaut `off`, ce prompt de secours n'est pas ajouté.

Exemple de contexte propre avec outils web :

```json
{
  "agents": {
    "defaults": {
      "turn_profile": {
        "enabled": true,
        "history": { "mode": "off" },
        "system_prompt": { "mode": "off" },
        "skills": { "mode": "off" },
        "tools": {
          "mode": "custom",
          "allow": ["web_search", "web_fetch"]
        }
      }
    }
  }
}
```

### Sources de Compétences

Par défaut, les compétences sont chargées depuis :

1. `~/.picoclaw/workspace/skills` (workspace)
2. `~/.picoclaw/skills` (global)
3. `<chemin-intégré-à-la-compilation>/skills` (intégré)

Pour les configurations avancées/de test, vous pouvez remplacer la racine des compétences builtin avec :

```bash
export PICOCLAW_BUILTIN_SKILLS=/path/to/skills
```

### Utiliser les Commandes Depuis les Canaux de Chat

Une fois les compétences installées, vous pouvez aussi les inspecter et les activer directement depuis un canal de chat :

- `/list skills` affiche les noms des compétences installées visibles pour l'agent courant.
- `/use <skill> <message>` force une compétence pour une seule requête.
- `/use <skill>` prépare cette compétence pour votre prochain message dans la meme conversation.
- `/use clear` annule une surcharge de compétence en attente creee via `/use <skill>`.
- `/btw <question>` pose une question annexe immediate sans modifier l'historique courant de la session. `/btw` est traite comme une requete directe sans outils et n'entre pas dans le flux normal d'execution des outils.

Exemples :

```text
/list skills
/use git explique comment squash les 3 derniers commits
/btw rappelle-moi ce qu'on a deja decide pour le plan de deploiement
/use italiapersonalfinance
dammi le ultime news
```

### Politique Unifiée d'Exécution des Commandes

- Les commandes slash génériques sont exécutées via un chemin unique dans `pkg/agent/loop.go` via `commands.Executor`.
- Les adaptateurs de canaux ne consomment plus les commandes génériques localement ; ils transmettent le texte entrant au chemin bus/agent. Telegram enregistre toujours automatiquement au démarrage les commandes prises en charge, comme `/start`, `/help`, `/show`, `/list`, `/use` et `/btw`.
- Une commande slash inconnue (par exemple `/foo`) passe au traitement LLM normal.
- Une commande enregistrée mais non prise en charge sur le canal actuel  renvoie une erreur explicite à l'utilisateur et arrête le traitement ultérieur.

### 🔒 Sandbox de Sécurité

PicoClaw s'exécute dans un environnement sandboxé par défaut. L'agent ne peut accéder aux fichiers et exécuter des commandes que dans le workspace configuré.

#### Configuration par Défaut

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.picoclaw/workspace",
      "restrict_to_workspace": true
    }
  }
}
```

| Option                  | Par défaut              | Description                                       |
| ----------------------- | ----------------------- | ------------------------------------------------- |
| `workspace`             | `~/.picoclaw/workspace` | Répertoire de travail de l'agent                  |
| `restrict_to_workspace` | `true`                  | Restreindre l'accès fichiers/commandes au workspace |

#### Outils Protégés

Lorsque `restrict_to_workspace: true`, les outils suivants sont sandboxés :

| Outil         | Fonction              | Restriction                                    |
| ------------- | --------------------- | ---------------------------------------------- |
| `read_file`   | Lire des fichiers     | Uniquement les fichiers dans le workspace      |
| `write_file`  | Écrire des fichiers   | Uniquement les fichiers dans le workspace      |
| `list_dir`    | Lister les répertoires| Uniquement les répertoires dans le workspace   |
| `edit_file`   | Modifier des fichiers | Uniquement les fichiers dans le workspace      |
| `append_file` | Ajouter aux fichiers  | Uniquement les fichiers dans le workspace      |
| `exec`        | Exécuter des commandes| Les chemins de commande doivent être dans le workspace |

#### Protection Exec Supplémentaire

Même avec `restrict_to_workspace: false`, l'outil `exec` bloque ces commandes dangereuses :

* `rm -rf`, `del /f`, `rmdir /s` — Suppression en masse
* `format`, `mkfs`, `diskpart` — Formatage de disque
* `dd if=` — Imagerie de disque
* Écriture vers `/dev/sd[a-z]` — Écritures directes sur disque
* `shutdown`, `reboot`, `poweroff` — Arrêt du système
* Fork bomb `:(){ :|:& };:`

### Contrôle d'Accès aux Fichiers

| Clé de configuration | Type | Par défaut | Description |
|----------------------|------|------------|-------------|
| `tools.allow_read_paths` | string[] | `[]` | Chemins supplémentaires autorisés en lecture en dehors du workspace |
| `tools.allow_write_paths` | string[] | `[]` | Chemins supplémentaires autorisés en écriture en dehors du workspace |

### Sécurité Exec

| Clé de configuration | Type | Par défaut | Description |
|----------------------|------|------------|-------------|
| `tools.exec.allow_remote` | bool | `false` | Autoriser l'outil exec depuis les canaux distants (Telegram/Feishu etc.) |
| `tools.exec.enable_deny_patterns` | bool | `true` | Activer l'interception des commandes dangereuses |
| `tools.exec.custom_deny_patterns` | string[] | `[]` | Patterns regex personnalisés à bloquer |
| `tools.exec.custom_allow_patterns` | string[] | `[]` | Patterns regex personnalisés à autoriser |

> **Note de sécurité :** La protection Symlink est activée par défaut — tous les chemins de fichiers sont résolus via `filepath.EvalSymlinks` avant la correspondance avec la liste blanche, empêchant les attaques d'évasion par symlink.

#### Limitation Connue : Processus Enfants des Outils de Build

Le garde de sécurité exec n'inspecte que la ligne de commande lancée directement par PicoClaw. Il n'inspecte pas récursivement les processus enfants générés par les outils de développement autorisés tels que `make`, `go run`, `cargo`, `npm run` ou les scripts de build personnalisés.

Cela signifie qu'une commande de niveau supérieur peut toujours compiler ou lancer d'autres binaires après avoir passé la vérification initiale du garde. En pratique, traitez les scripts de build, les Makefiles, les scripts de packages et les binaires générés comme du code exécutable nécessitant le même niveau de revue qu'une commande shell directe.

Pour les environnements à haut risque :

* Examinez les scripts de build avant l'exécution.
* Préférez l'approbation/revue manuelle pour les workflows de compilation et d'exécution.
* Exécutez PicoClaw dans un conteneur ou une VM si vous avez besoin d'une isolation plus forte que celle fournie par le garde intégré.

#### Exemples d'Erreurs

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (path outside working dir)}
```

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (dangerous pattern detected)}
```

#### Désactiver les Restrictions (Risque de Sécurité)

Si vous avez besoin que l'agent accède à des chemins en dehors du workspace :

**Méthode 1 : Fichier de configuration**

```json
{
  "agents": {
    "defaults": {
      "restrict_to_workspace": false
    }
  }
}
```

**Méthode 2 : Variable d'environnement**

```bash
export PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE=false
```

> ⚠️ **Avertissement** : Désactiver cette restriction permet à l'agent d'accéder à n'importe quel chemin sur votre système. À utiliser avec précaution dans des environnements contrôlés uniquement.

#### Cohérence des Limites de Sécurité

Le paramètre `restrict_to_workspace` s'applique de manière cohérente à tous les chemins d'exécution :

| Chemin d'exécution | Limite de sécurité               |
| ------------------ | -------------------------------- |
| Main Agent         | `restrict_to_workspace` ✅       |
| Subagent / Spawn   | Hérite de la même restriction ✅ |
| Heartbeat tasks    | Hérite de la même restriction ✅ |

Tous les chemins partagent la même restriction de workspace — il n'y a aucun moyen de contourner la limite de sécurité via les subagents ou les tâches planifiées.

### Heartbeat (Tâches Périodiques)

PicoClaw peut effectuer des tâches périodiques automatiquement. Créez un fichier `HEARTBEAT.md` dans votre workspace :

```markdown
# Periodic Tasks

- Check my email for important messages
- Review my calendar for upcoming events
- Check the weather forecast
```

L'agent lira ce fichier toutes les 30 minutes (configurable) et exécutera toutes les tâches en utilisant les outils disponibles.

#### Tâches Asynchrones avec Spawn

Pour les tâches longues (recherche web, appels API), utilisez l'outil `spawn` pour créer un **subagent** :

```markdown
# Tâches Périodiques

## Tâches Rapides (répondre directement)

- Indiquer l'heure actuelle

## Tâches Longues (utiliser spawn pour l'asynchrone)

- Rechercher les actualités IA sur le web et résumer
- Vérifier les e-mails et signaler les messages importants
```

**Comportements clés :**

| Fonctionnalité   | Description                                                        |
| ---------------- | ------------------------------------------------------------------ |
| **spawn**        | Crée un subagent asynchrone, ne bloque pas le heartbeat            |
| **Contexte indépendant** | Le subagent a son propre contexte, sans historique de session |
| **message tool** | Le subagent communique directement avec l'utilisateur              |
| **Non-bloquant** | Après le spawn, le heartbeat continue vers la tâche suivante       |

#### Flux de Communication du Subagent

```
Heartbeat déclenché
    ↓
Agent lit HEARTBEAT.md
    ↓
Tâche longue : spawn subagent
    ↓                           ↓
Continue tâche suivante    Subagent travaille indépendamment
    ↓                           ↓
Toutes tâches terminées    Subagent utilise "message" tool
    ↓                           ↓
Répond HEARTBEAT_OK        Utilisateur reçoit le résultat
```

**Configuration :**

```json
{
  "heartbeat": {
    "enabled": true,
    "interval": 30
  }
}
```

| Option     | Défaut | Description                              |
| ---------- | ------ | ---------------------------------------- |
| `enabled`  | `true` | Activer/désactiver le heartbeat          |
| `interval` | `30`   | Intervalle en minutes (minimum : 5)      |

**Variables d'environnement :**

* `PICOCLAW_HEARTBEAT_ENABLED=false` pour désactiver
* `PICOCLAW_HEARTBEAT_INTERVAL=60` pour changer l'intervalle

### Model providers

This fork supports only OpenAI (Responses / Chat Completions) and Anthropic Messages. For compatible endpoints, API keys, streaming and login, see the maintained [provider guide](providers.md) or [中文说明](providers.zh.md).

### Tâches Planifiées / Rappels

PicoClaw supporte les tâches planifiées via l'outil `cron`. L'agent peut définir, lister et annuler des rappels ou tâches récurrentes.

```json
{
  "tools": {
    "cron": {
      "enabled": true,
      "exec_timeout_minutes": 5
    }
  }
}
```

Les tâches planifiées persistent après redémarrage dans `~/.picoclaw/workspace/cron/`.

### Sujets Avancés

| Sujet | Description |
| ----- | ----------- |
| [Système de Hooks](../architecture/hooks/README.md) | Hooks événementiels : observateurs, intercepteurs, hooks d'approbation |
| [Steering](../architecture/steering.md) | Injecter des messages dans une boucle agent en cours d'exécution |
| [SubTurn](../architecture/subturn.md) | Coordination de subagents, contrôle de concurrence, cycle de vie |
| [Gestion du Contexte](../architecture/agent-refactor/context.md) | Détection des limites de contexte, compression |
