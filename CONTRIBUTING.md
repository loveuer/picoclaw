# Contributing to this fork

This repository is [loveuer/picoclaw](https://github.com/loveuer/picoclaw), a smaller fork of [sipeed/picoclaw](https://github.com/sipeed/picoclaw). Its supported features and release process differ from upstream. Start with the [README](README.md) and [configuration example](config/config.example.json).

## Scope

Keep the CLI, Gateway, supported message channels, OpenAI and Anthropic protocols, tools, MCP, and Skills maintainable. Prefer optional Skills or external tools for specialized integrations such as office files and hardware. Discuss substantial changes before adding dependencies or reintroducing removed features.

The current channels are Weixin, WeCom, Feishu, DingTalk, QQ, OneBot, Telegram, and MQTT. Native Gemini, Azure, Bedrock, vendor presets, CLI model proxies, WebUI, and built-in hardware tools have been removed. Compatible model endpoints use an explicit `openai` or `anthropic` provider and `api_base`.

## Development

Install Git, Make, and a Go toolchain satisfying [go.mod](go.mod), currently Go 1.25.13 or later. Clone this fork, or your fork of it:

```bash
git clone https://github.com/loveuer/picoclaw.git
cd picoclaw
make build
./build/picoclaw version
```

`make build` generates embedded resources, uses `stdjson`, and disables CGO by default. Keep the Go module path `github.com/sipeed/picoclaw`; it remains the internal import path.

Before submitting changes, run the checks relevant to the change:

```bash
go generate ./...
go test -tags stdjson ./...
go vet -tags stdjson ./...
make lint-docs
make lint
```

CI also runs vulnerability checks and Docker integration suites. For integration setup and suite selection, see [integration/README.md](integration/README.md).

Keep changes focused. Describe the problem, resulting behavior, and validation in the PR. Never include credentials, personal configuration, or user workspace data. Preserve third-party license and attribution files, including those bundled with the Office Skills.

## Configuration and documentation

When removing a feature, clean up its registration, configuration, tests, dependencies, examples, and documentation. Consider existing user configurations so upgrades do not unexpectedly fail. Channels use `channel_list` with channel-specific values under `settings`; `.security.yml` may override values from the main JSON file.

Maintain the root README as the project overview. Do not restore duplicate upstream README translations. Keep specialized guides beside their translations and update links when moving or deleting documents. See [docs/README.md](docs/README.md).

## Releases

CI builds Linux amd64. Releases are published manually through the `Release` workflow, using an existing stable tag or replacing the `nightly` prerelease. Assets are a Linux amd64 archive and `checksums.txt`; there are no scheduled nightly builds, Docker registry uploads, or deb/rpm packages. See the [README release instructions](README.md#检查与发布).

The updater defaults to this fork's GitHub Releases. Changing the default source or publication format requires reviewing updater compatibility.
