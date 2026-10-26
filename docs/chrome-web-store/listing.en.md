# Chrome Web Store copy · English

Version: 0.1.7. Listing locale: English / `en`. Field notes are not part of the description to paste.

## Name

Passpine

## Short description

Your native Passpine vault, securely connected to your browser.

This is the packaged `extensionDescription` in `_locales/en/messages.json`.

## Detailed description

Passpine connects your local password vault to Chrome so you can find accounts, copy credentials, and fill login forms from your browser.

Install and run the Passpine desktop client first, create or open a vault, and register its browser connection. New users do not need an existing password-store or GPG key: the client can create a pass-compatible vault and generate a passphrase-protected OpenPGP private key.

Features:
• Search your local vault and view account names, usernames, passwords, websites, notes, and available one-time codes. Manage custom fields in the account editor.
• Click an account to fill the current page's username, password, and supported one-time-code fields. Filling does not submit the form.
• Associate accounts with the current website to find them next time. Associations use the exact hostname and do not automatically extend to other subdomains.
• Create and edit accounts, generate passwords within the account form, and move deleted accounts to the client's encrypted trash.
• Share the desktop client's unlock session. When locked, use system authentication on supported and configured devices, or unlock in the client.
• Choose English, Simplified Chinese, or the system language. Customize themes, appearance, text size, and keyboard shortcuts.

Data handling: the extension communicates with the local Passpine client through Native Messaging to retrieve or save credentials for your actions. It reads the current tab's address to match accounts. When you request filling, it inspects login inputs and gives the selected credentials to that page. Website associations and extension preferences stay in local browser storage. If you choose Git synchronization, the desktop client connects to your configured repository. Entry contents in that repository use OpenPGP encryption; file paths and Git metadata remain visible.

Requirements and limits: the desktop client must keep running; the extension is not a standalone vault. Browser internal pages, cross-origin frames, closed Shadow DOM, and some custom forms may not support filling. Check the website before filling. HTTP pages do not provide HTTPS transport protection. System authentication availability depends on the operating system and client configuration.

Project and desktop client: https://github.com/knothhe/passpine
Help and issue reporting: https://github.com/knothhe/passpine/issues

## Other dashboard fields

| Field | Suggested value |
| --- | --- |
| Category | Productivity; verify against the current dashboard options |
| Homepage | https://passpine.knothhe.com/ |
| Support URL | https://github.com/knothhe/passpine/issues |
| Privacy policy | https://passpine.knothhe.com/privacy (see [PRIVACY.md](PRIVACY.md)) |
| Distribution / pricing | Publisher decision; repository code does not establish future commercial terms |

Confirm that the repository and Issues page are publicly accessible before submitting. Do not use a local file path or unpublished branch URL as the live privacy-policy URL.
