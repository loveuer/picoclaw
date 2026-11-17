# Platforms and hardware boundaries

The automated build and release target for this fork is **Linux amd64**. See the [README](../../README.md) for source builds and release packaging.

```bash
make build GOOS=linux GOARCH=amd64
# build/picoclaw-linux-amd64
```

The Makefile retains other cross-compilation helpers, but they are not part of the current release workflow or a verified device compatibility list. Validate builds, channel SDK support, and runtime behavior on the target platform before deploying. In particular, the Feishu SDK is unavailable on 32-bit targets.

Built-in I²C, SPI, serial tools, USB device monitoring, and the hardware Skill have been removed. Add hardware integrations through optional Skills and external tools. This does not prevent the Gateway or CLI from running on a supported host with hardware attached.

Do not install upstream release archives as updates for this fork. The Office Skills use a separate Docker runtime; see the [Office guide](office-skills.zh.md).
