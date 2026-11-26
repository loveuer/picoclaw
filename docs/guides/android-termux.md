# Android Termux

> Back to [Guides](README.md)

For an ARM64 Android device running Termux, download `picoclaw_Android_arm64.tgz` from a [stable release of this fork](https://github.com/loveuer/picoclaw/releases/latest), extract it inside Termux, and run `./picoclaw`.

To build from source, run `make build-android-arm64` to produce `build/picoclaw-android-arm64`. The release workflow cross-compiles the binary. Check runtime compatibility on your device.

Model protocols and configuration are described in the [provider guide](providers.md).
