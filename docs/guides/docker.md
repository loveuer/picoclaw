# Docker builds for this fork

> Back to [README](../../README.md)

This fork does not publish Docker images. Build from this repository. The Compose image name `docker.io/sipeed/picoclaw:latest` is a historical local tag; pulling it downloads the upstream image.

```bash
git clone https://github.com/loveuer/picoclaw.git
cd picoclaw
docker compose -f docker/docker-compose.yml --profile gateway build
docker compose -f docker/docker-compose.yml run --rm \
  --entrypoint picoclaw picoclaw-agent onboard
```

Edit `docker/data/config.json` and, where applicable, `docker/data/.security.yml`. Configure an OpenAI or Anthropic model and enable the required channels. Use `channel_list.<name>.settings` for channel-specific fields. See the [configuration example](../../config/config.example.json) and [provider guide](providers.md).

```bash
docker compose -f docker/docker-compose.yml --profile gateway up -d --no-build --pull never
docker compose -f docker/docker-compose.yml logs -f picoclaw-gateway
docker compose -f docker/docker-compose.yml --profile gateway down

# CLI instead of Gateway
docker compose -f docker/docker-compose.yml run --rm --pull never picoclaw-agent -m "Hello"
```

`docker/data/` is mounted at `/root/.picoclaw`; stopping the container preserves it. Gateway binds locally within the container by default, and Compose does not publish its port. Accessing `/health` or `/ready` from the host requires both `gateway.host: "0.0.0.0"` and a port mapping. There is no WebUI or generic REST chat endpoint.

After updating the local source, rebuild and start the container again:

```bash
git pull --ff-only
docker compose -f docker/docker-compose.yml --profile gateway build
docker compose -f docker/docker-compose.yml --profile gateway up -d --no-build --pull never
```

Do not use `docker compose pull` to update this fork. Existing workspace Skills are not overwritten by image updates. The ordinary Gateway image does not provide Docker daemon access for the Office Skills; use a host deployment with the [dedicated Office runtime](office-skills.zh.md) when needed.

The full/heavy Docker variants remain local build options, outside the automated release workflow. See the [Chinese guide](docker.zh.md) for details.
