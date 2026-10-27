# Passpine

English | [简体中文](README-zh_CN.md)

A local password manager compatible with [pass](https://www.passwordstore.org/), with a native desktop app for macOS, Windows and Linux, plus browser extensions. Includes password generation, TOTP, Git sync and web form filling.

Create an OpenPGP-encrypted vault without installing GnuPG, or import an existing pass vault using system GPG or a private key file.

## Install

**macOS** — Apple Silicon, macOS 13 or later:

```sh
brew install --cask knothhe/tap/passpine
```

**Linux** — x86_64:

```sh
curl -fsSL https://raw.githubusercontent.com/knothhe/passpine/main/scripts/install-linux.sh | sh
```

**Windows** — x86_64: download the installer from [GitHub Releases](https://github.com/knothhe/passpine/releases). Manual downloads for macOS and Linux, and Chrome/Firefox extension packages, are also available there.

See the [installation guide](docs/install-local.md) for platform requirements, browser setup and uninstalling. The macOS app is not yet notarized; the Windows installer has no Authenticode signature.

## Get started

1. Open the app and create a vault, open a local pass vault, or clone a Git repository. New vaults require a passphrase of at least 12 characters.
2. Export and verify your recovery private key. Keep backups of your vault and recovery key, and store your passphrase safely.
3. Enable Git sync or browser integration in settings as needed. Keep the desktop app running when using the extension.

A pass-compatible CLI is available on macOS and Linux, for example `passpine show -c email/work`. See the [CLI guide](docs/cli.md).

## Development

Requires Make, Go 1.27.1+, Node.js 22.12+ and pnpm 12.11.2. See the installation guide for platform-specific dependencies. Run from the repository root:

```sh
make                     # Build the Dev client and Chrome/Firefox extensions
make client:install      # Install the Dev client and register browser integration
make help                # List commands and options
pnpm run preview         # Run the desktop demo
pnpm --dir browser install --frozen-lockfile
pnpm run preview:browser # Browser demo at http://127.0.0.1:5174/
pnpm test                # Run checks and tests
```

The Dev client has separate app and data directories. Locally loaded extensions default to the release client; select Dev in extension settings to connect to the Dev client. Both demos use fictional data; the desktop demo passphrase is `Passpine-demo-2026`.

## Documentation

Detailed guides are currently in Chinese:

- [Installation and browser setup](docs/install-local.md) · [CLI](docs/cli.md)
- [Browser extension](browser/README.md) · [Website](website/README.md)
- [Desktop settings](docs/desktop-settings.md) · [R2 / S3 configuration sync](docs/config-sync.md)
- [Architecture and security](docs/ARCHITECTURE.md) · [Automatic updates](docs/updates.md)
- [Versioning and releases](docs/versioning.md) · [Validation](docs/VALIDATION.md)

## License

[MIT](LICENSE)
