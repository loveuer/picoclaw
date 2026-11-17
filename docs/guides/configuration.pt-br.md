# ⚙️ Guia de Configuração

> Fork configuration: `PICOCLAW_HOME` also changes the default config path; `PICOCLAW_CONFIG` overrides that path. For current model protocols and channel settings, use the [README](../../README.md) and [configuration example](../../config/config.example.json).

> Voltar ao [README](../../README.md)

## ⚙️ Configuração

Arquivo de configuração: `~/.picoclaw/config.json`

### Variáveis de Ambiente

Você pode substituir os caminhos padrão usando variáveis de ambiente. Isso é útil para instalações portáteis, implantações em contêineres ou execução do picoclaw como serviço do sistema. Essas variáveis são independentes e controlam caminhos diferentes.

| Variável          | Descrição                                                                                                                             | Caminho Padrão              |
|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------|---------------------------|
| `PICOCLAW_CONFIG` | Substitui o caminho para o arquivo de configuração. Isso indica diretamente ao picoclaw qual `config.json` carregar, ignorando todos os outros locais. | `~/.picoclaw/config.json` |
| `PICOCLAW_HOME`   | Substitui o diretório raiz para dados do picoclaw. Isso altera o local padrão do `workspace` e outros diretórios de dados.          | `~/.picoclaw`             |

**Exemplos:**

```bash
# Executar picoclaw usando um arquivo de configuração específico
# O caminho do workspace será lido de dentro desse arquivo de configuração
PICOCLAW_CONFIG=/etc/picoclaw/production.json picoclaw gateway

# Executar picoclaw com todos os dados armazenados em /opt/picoclaw
# Config: /opt/picoclaw/config.json (PICOCLAW_HOME=/opt/picoclaw)
# O workspace será criado em /opt/picoclaw/workspace
PICOCLAW_HOME=/opt/picoclaw picoclaw agent

# Usar ambos para uma configuração totalmente personalizada
PICOCLAW_HOME=/srv/picoclaw PICOCLAW_CONFIG=/srv/picoclaw/main.json picoclaw gateway
```

### Nível de Log do Gateway

`gateway.log_level` controla a verbosidade dos logs do Gateway, configurável em `config.json`:

```json
{
  "gateway": {
    "log_level": "warn"
  }
}
```

O valor padrão é `warn`. Valores suportados: `debug`, `info`, `warn`, `error`, `fatal`.

Também pode ser substituído pela variável de ambiente: `PICOCLAW_LOG_LEVEL=info`

### Layout do Workspace

O PicoClaw armazena dados no seu workspace configurado (padrão: `~/.picoclaw/workspace`):

```
~/.picoclaw/workspace/
├── sessions/          # Sessões de conversa e histórico
├── memory/           # Memória de longo prazo (MEMORY.md)
├── state/            # Estado persistente (último canal, etc.)
├── cron/             # Banco de dados de tarefas agendadas
├── skills/           # Skills personalizadas
├── AGENT.md          # Guia de comportamento do agente
├── HEARTBEAT.md      # Prompts de tarefas periódicas (verificados a cada 30 min)
├── IDENTITY.md       # Identidade do agente
├── SOUL.md           # Alma do agente
└── USER.md           # Preferências do usuário
```

> **Nota:** Alterações em `AGENT.md`, `SOUL.md`, `USER.md` e `memory/MEMORY.md` são detectadas automaticamente em tempo de execução via rastreamento de data de modificação (mtime). **Não é necessário reiniciar o gateway** após editar esses arquivos — o agente carrega o novo conteúdo na próxima requisição.

### Política de contexto da requisição

`turn_profile` é uma política opcional em `agents.defaults.turn_profile` para controlar qual contexto cada novo turno carrega: histórico, prompt de sistema, prompts de skills e ferramentas permitidas. Sem essa configuração, ou com `"enabled": false`, o PicoClaw mantém o comportamento normal. Com `"enabled": true`, a política abaixo se aplica a cada novo turno.

Todos os blocos usam os mesmos valores de `mode`:

| Mode | Significado |
| --- | --- |
| `default` | Mantém o comportamento normal do PicoClaw. Blocos ausentes ou sem `mode` são tratados como `default`. |
| `off` | Desativa esse bloco para o turno. |
| `custom` | Usa uma lista de permissão. Nesta versão, `custom` só é suportado para `skills` e `tools`; usá-lo em `history` ou `system_prompt` gera erro de validação. |

Blocos disponíveis:

| Bloco | O que controla |
| --- | --- |
| `history` | Leitura de histórico e resumo, gravação de mensagens de usuário/assistente/ferramenta, ingestão de contexto, compactação e resumo. |
| `system_prompt` | Injeção da identidade padrão do PicoClaw, instruções do workspace, memória, contexto de execução e resumo. Prompts de sistema externos ainda são permitidos quando este bloco está `off`. |
| `skills` | Catálogo de skills e conteúdo de skills ativas no prompt. `custom.allow` mantém apenas os nomes listados. |
| `tools` | Ferramentas visíveis ao modelo e permitidas na execução. `custom.allow` mantém apenas ferramentas registradas e listadas. |

Quando `system_prompt.mode` é `off`, ferramentas continuam visíveis e nenhum prompt de sistema externo é fornecido, o PicoClaw reutiliza sua regra existente de uso de ferramentas como prompt mínimo de fallback. Se `tools.mode` é `off`, esse fallback não é adicionado.

Exemplo de contexto limpo com ferramentas web:

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

### Fontes de Skills

Por padrão, as skills são carregadas de:

1. `~/.picoclaw/workspace/skills` (workspace)
2. `~/.picoclaw/skills` (global)
3. `<caminho-embutido-na-compilação>/skills` (embutido)

Para configurações avançadas/de teste, você pode substituir o diretório raiz de skills builtin com:

```bash
export PICOCLAW_BUILTIN_SKILLS=/path/to/skills
```

### Usando Skills e Comandos em Canais de Chat

Depois que as skills estiverem instaladas, voce pode inspeciona-las e aplica-las diretamente de um canal de chat:

- `/list skills` mostra os nomes das skills instaladas visiveis para o agente atual.
- `/use <skill> <message>` força uma skill para uma unica requisicao.
- `/use <skill>` prepara essa skill para a sua proxima mensagem no mesmo chat.
- `/use clear` cancela uma substituicao pendente criada por `/use <skill>`.
- `/btw <question>` faz uma pergunta lateral imediata sem alterar o historico atual da sessao. `/btw` e tratado como uma consulta direta sem ferramentas e nao entra no fluxo normal de execucao de ferramentas.

Exemplos:

```text
/list skills
/use git explique como fazer squash dos ultimos 3 commits
/btw me relembre o que ja decidimos sobre o plano de deploy
/use italiapersonalfinance
dammi le ultime news
```

### Política Unificada de Execução de Comandos

- Comandos slash genéricos são executados através de um único caminho em `pkg/agent/loop.go` via `commands.Executor`.
- Os adaptadores de canal não consomem mais comandos genéricos localmente; eles encaminham o texto de entrada para o caminho bus/agent. O Telegram ainda registra automaticamente na inicialização comandos suportados como `/start`, `/help`, `/show`, `/list`, `/use` e `/btw`.
- Comando slash desconhecido (por exemplo `/foo`) passa para o processamento normal do LLM.
- Comando registrado mas não suportado no canal atual  retorna um erro explícito ao usuário e interrompe o processamento.

### 🔒 Sandbox de Segurança

O PicoClaw é executado em um ambiente sandbox por padrão. O agente só pode acessar arquivos e executar comandos dentro do workspace configurado.

#### Configuração Padrão

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

| Opção                   | Padrão                  | Descrição                                 |
| ----------------------- | ----------------------- | ----------------------------------------- |
| `workspace`             | `~/.picoclaw/workspace` | Diretório de trabalho do agente           |
| `restrict_to_workspace` | `true`                  | Restringir acesso a arquivos/comandos ao workspace |

#### Ferramentas Protegidas

Quando `restrict_to_workspace: true`, as seguintes ferramentas são isoladas:

| Ferramenta    | Função           | Restrição                              |
| ------------- | ---------------- | -------------------------------------- |
| `read_file`   | Ler arquivos     | Apenas arquivos dentro do workspace    |
| `write_file`  | Escrever arquivos| Apenas arquivos dentro do workspace    |
| `list_dir`    | Listar diretórios| Apenas diretórios dentro do workspace  |
| `edit_file`   | Editar arquivos  | Apenas arquivos dentro do workspace    |
| `append_file` | Anexar a arquivos| Apenas arquivos dentro do workspace    |
| `exec`        | Executar comandos| Caminhos de comando devem estar dentro do workspace |

#### Proteção Adicional do Exec

Mesmo com `restrict_to_workspace: false`, a ferramenta `exec` bloqueia estes comandos perigosos:

* `rm -rf`, `del /f`, `rmdir /s` — Exclusão em massa
* `format`, `mkfs`, `diskpart` — Formatação de disco
* `dd if=` — Imagem de disco
* Escrita em `/dev/sd[a-z]` — Escritas diretas em disco
* `shutdown`, `reboot`, `poweroff` — Desligamento do sistema
* Fork bomb `:(){ :|:& };:`

### Controle de Acesso a Arquivos

| Config Key | Type | Default | Description |
|------------|------|---------|-------------|
| `tools.allow_read_paths` | string[] | `[]` | Additional paths allowed for reading outside workspace |
| `tools.allow_write_paths` | string[] | `[]` | Additional paths allowed for writing outside workspace |

### Segurança do Exec

| Config Key | Type | Default | Description |
|------------|------|---------|-------------|
| `tools.exec.allow_remote` | bool | `false` | Allow exec tool from remote channels (Telegram/Feishu etc.) |
| `tools.exec.enable_deny_patterns` | bool | `true` | Enable dangerous command interception |
| `tools.exec.custom_deny_patterns` | string[] | `[]` | Custom regex patterns to block |
| `tools.exec.custom_allow_patterns` | string[] | `[]` | Custom regex patterns to allow |

> **Nota de Segurança:** A proteção contra symlinks é habilitada por padrão — todos os caminhos de arquivo são resolvidos através de `filepath.EvalSymlinks` antes da correspondência com a whitelist, prevenindo ataques de escape via symlink.

#### Limitação Conhecida: Processos Filhos de Ferramentas de Build

O guard de segurança do exec inspeciona apenas a linha de comando que o PicoClaw executa diretamente. Ele não inspeciona recursivamente processos filhos gerados por ferramentas de desenvolvimento permitidas como `make`, `go run`, `cargo`, `npm run` ou scripts de build personalizados.

Isso significa que um comando de nível superior ainda pode compilar ou executar outros binários após passar pela verificação inicial do guard. Na prática, trate scripts de build, Makefiles, scripts de pacotes e binários gerados como código executável que precisa do mesmo nível de revisão que um comando shell direto.

Para ambientes de maior risco:

* Revise scripts de build antes da execução.
* Prefira aprovação/revisão manual para fluxos de trabalho de compilação e execução.
* Execute o PicoClaw dentro de um contêiner ou VM se precisar de isolamento mais forte do que o guard integrado oferece.

#### Exemplos de Erro

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (path outside working dir)}
```

```
[ERROR] tool: Tool execution failed
{tool=exec, error=Command blocked by safety guard (dangerous pattern detected)}
```

#### Desabilitando Restrições (Risco de Segurança)

Se você precisar que o agente acesse caminhos fora do workspace:

**Método 1: Arquivo de configuração**

```json
{
  "agents": {
    "defaults": {
      "restrict_to_workspace": false
    }
  }
}
```

**Método 2: Variável de ambiente**

```bash
export PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE=false
```

> ⚠️ **Aviso**: Desabilitar esta restrição permite que o agente acesse qualquer caminho no seu sistema. Use com cautela apenas em ambientes controlados.

#### Consistência do Limite de Segurança

A configuração `restrict_to_workspace` se aplica consistentemente em todos os caminhos de execução:

| Caminho de Execução | Limite de Segurança          |
| -------------------- | ---------------------------- |
| Main Agent           | `restrict_to_workspace` ✅   |
| Subagent / Spawn     | Herda a mesma restrição ✅   |
| Heartbeat tasks      | Herda a mesma restrição ✅   |

Todos os caminhos compartilham a mesma restrição de workspace — não há como contornar o limite de segurança através de subagentes ou tarefas agendadas.

### Heartbeat (Tarefas Periódicas)

O PicoClaw pode executar tarefas periódicas automaticamente. Crie um arquivo `HEARTBEAT.md` no seu workspace:

```markdown
# Tarefas Periódicas

- Verificar meu e-mail para mensagens importantes
- Revisar meu calendário para eventos próximos
- Verificar a previsão do tempo
```

O agente lerá este arquivo a cada 30 minutos (configurável) e executará quaisquer tarefas usando as ferramentas disponíveis.

#### Tarefas Assíncronas com Spawn

Para tarefas de longa duração (busca na web, chamadas de API), use a ferramenta `spawn` para criar um **subagente**:

```markdown
# Tarefas Periódicas

## Tarefas Rápidas (responder diretamente)

- Informar a hora atual

## Tarefas Longas (usar spawn para assíncrono)

- Pesquisar notícias de IA na web e resumir
- Verificar e-mails e reportar mensagens importantes
```

**Comportamentos principais:**

| Funcionalidade   | Descrição                                                          |
| ---------------- | ------------------------------------------------------------------ |
| **spawn**        | Cria subagente assíncrono, não bloqueia o heartbeat                |
| **Contexto independente** | Subagente tem seu próprio contexto, sem histórico de sessão |
| **message tool** | Subagente comunica diretamente com o usuário via message tool      |
| **Não-bloqueante** | Após o spawn, o heartbeat continua para a próxima tarefa         |

#### Fluxo de Comunicação do Subagente

```
Heartbeat disparado
    ↓
Agent lê HEARTBEAT.md
    ↓
Tarefa longa: spawn subagente
    ↓                           ↓
Continua próxima tarefa    Subagente trabalha independentemente
    ↓                           ↓
Todas tarefas concluídas   Subagente usa ferramenta "message"
    ↓                           ↓
Responde HEARTBEAT_OK      Usuário recebe resultado diretamente
```

**Configuração:**

```json
{
  "heartbeat": {
    "enabled": true,
    "interval": 30
  }
}
```

| Opção      | Padrão | Descrição                              |
| ---------- | ------ | -------------------------------------- |
| `enabled`  | `true` | Ativar/desativar heartbeat             |
| `interval` | `30`   | Intervalo em minutos (mínimo: 5)       |

**Variáveis de ambiente:**

* `PICOCLAW_HEARTBEAT_ENABLED=false` para desativar
* `PICOCLAW_HEARTBEAT_INTERVAL=60` para alterar o intervalo

### Model providers

This fork supports only OpenAI (Responses / Chat Completions) and Anthropic Messages. For compatible endpoints, API keys, streaming and login, see the maintained [provider guide](providers.md) or [中文说明](providers.zh.md).

### Tarefas Agendadas / Lembretes

PicoClaw suporta tarefas agendadas via ferramenta `cron`.

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

As tarefas agendadas persistem após reinicializações em `~/.picoclaw/workspace/cron/`.

### Tópicos Avançados

| Tópico | Descrição |
| ------ | --------- |
| [Sistema de Hooks](../architecture/hooks/README.md) | Hooks orientados a eventos: observadores, interceptores, hooks de aprovação |
| [Steering](../architecture/steering.md) | Injetar mensagens em um loop de agente em execução |
| [SubTurn](../architecture/subturn.md) | Coordenação de subagentes, controle de concorrência, ciclo de vida |
| [Gerenciamento de Contexto](../architecture/agent-refactor/context.md) | Detecção de limites de contexto, compressão |
