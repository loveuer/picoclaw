# Office file skills

[中文说明](office-skills.zh.md)

PicoClaw bundles `docx`, `xlsx`, and `pptx` skills adapted from Anthropic's document skills. Each directory retains its source revision and Apache-2.0 license. They operate on local files, not Feishu online documents.

## Prepare the runtime

Run as the same user that runs the gateway. Replace the default workspace path if customized:

```bash
picoclaw skills install-builtin docx xlsx pptx
bash ~/.picoclaw/workspace/tools/office/office.sh setup
bash ~/.picoclaw/workspace/tools/office/office.sh check
```

A new `onboard` workspace already includes the files, but still requires `setup`. The host needs Bash and access to an existing Docker daemon. The script does not install Docker, use sudo, restart the gateway, or change service permissions. Linux is the validated platform.

`install-builtin` with no names installs all embedded skills. Existing directories are skipped. `--force` replaces selected skill directories and the shared office runtime, so preserve local changes first. Other custom skills remain untouched. Upgrade the binary before updating its embedded resources.

The image includes Python, Node.js, LibreOffice, Pandoc, Poppler, and Chinese fonts. Python/npm packages are version-locked; the base image and Debian system packages follow their build-time repositories. Build input changes produce a new image tag. Repeated setup uses Docker's build cache.

## Execute commands

```bash
bash ~/.picoclaw/workspace/tools/office/office.sh run -- \
  pandoc -t markdown /home/your-user/.picoclaw/workspace/report.docx
```

Pass commands and arguments separately; the wrapper does not evaluate shell expressions. Use `run -- bash -c '...'` for shell logic and quote filenames. Helper script paths refer to the corresponding skill's `scripts/` directory; use absolute paths. A working directory inside the workspace is preserved; otherwise commands start at the workspace root.

Only the workspace is mounted, at its original absolute path. Copy outside inputs into it first. Each command uses a temporary container with the current UID/GID and no network. The host HOME, configuration secrets, and Docker socket are not mounted. Output files remain in the workspace. Avoid commas in workspace paths because of Docker's bind-mount syntax.

## Capabilities and limits

- Word: create documents with docx-js, extract text with Pandoc, edit OOXML, and handle comments or tracked changes using helper scripts.
- Excel: use openpyxl/pandas and recalculate formulas with LibreOffice. Check calculated values; external links, macros, and advanced functions need special care.
- PowerPoint: create with PptxGenJS, edit templates, validate packages, and render previews.

Use `Noto Sans CJK SC` or `Noto Serif CJK SC` for Chinese text. Convert legacy `.doc`/`.ppt` files to `.docx`/`.pptx` first. LibreOffice rendering and formula behavior can differ from Microsoft Office. A successful conversion does not replace visual review or checking business calculations. Report when image inspection is unavailable.

This feature does not change existing channel file upload/download/delivery capabilities.

## Verification and troubleshooting

Run the included smoke test with a new output directory:

```bash
bash ~/.picoclaw/workspace/tools/office/office.sh run -- \
  python /home/your-user/.picoclaw/workspace/tools/office/smoke_test.py \
  /home/your-user/.picoclaw/workspace/office-check-001
```

It creates and edits Chinese Word, Excel, and PowerPoint fixtures, checks comments and formula results, and renders PDFs/previews. It makes no model API calls.

If Docker is unavailable, prepare it as an operator. If the runtime image is missing, run `setup`; normal document tasks never build or install dependencies automatically. Run `check` after upgrading resources. If tool policy rejects execution, inspect the existing policy rather than globally disabling restrictions. Large conversions may require a longer task execution timeout.
