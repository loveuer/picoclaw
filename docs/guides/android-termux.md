# Android Termux

> Back to [Guides](README.md)

This fork currently publishes only Linux amd64 archives. There is no Android or Linux ARM64 release asset; the previous upstream download commands do not install this fork.

The Makefile retains `make build-android-arm64`, which produces `build/picoclaw-android-arm64` from this repository. Android builds and runtime behavior are outside the current CI and have not been verified for this fork. Check the target environment and dependencies before using them.

For the maintained installation path, use [Linux amd64 or a local source build](../../README.md#从源码构建). Model protocols and configuration are described in the [provider guide](providers.md).
