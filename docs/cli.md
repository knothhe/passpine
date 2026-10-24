# 命令行

Passpine 客户端可执行文件同时提供兼容 [pass](https://www.passwordstore.org/) 的命令行。命令以 `passpine` 开始（Dev 版本为 `passpine-dev`）；不带参数或只带 `--preview`、`--data-dir` 等选项运行时仍打开桌面客户端。

```sh
passpine ls
passpine show -c email/work
passpine generate -n web/example 32
```

## 安装与 pass 别名

macOS / Linux 可在客户端 **设置 → 命令行** 中管理命令链接。进入命令行设置会检查当前应用的主命令与 `pass` 别名，点击右上角的**刷新状态**按钮可重新读取；**安装命令** 默认只安装主命令，勾选 **同时安装 pass 别名** 后再安装，会同时创建 `pass`。更改此选择本身不会修改命令链接，也不会删除已有别名。

**卸载** 会移除指向当前应用的主命令和 `pass` 符号链接，保留其他程序、其他通道的链接和应用可执行文件。Dev 版使用 `passpine-dev`，正式版使用 `passpine`；两者共用的 `pass` 别名不会互相覆盖。预览模式不操作命令链接；Windows 显示暂不支持说明。

Linux 安装脚本已将 `passpine` 放入 `~/.local/bin`，设置中会将该可执行文件标为“已安装”，状态提示会说明“由应用安装器提供”，不会通过此页面的卸载操作删除它。macOS 也可以在终端运行一次应用包内的程序以创建命令链接（按实际安装位置调整路径）：

```sh
"/Applications/Passpine.app/Contents/MacOS/passpine" cli install
```

是否使用 `pass` 别名由用户选择，默认不创建：

```sh
passpine cli install --pass-alias     # 创建 pass → passpine 链接
passpine cli uninstall --pass-alias   # 只移除 pass 别名
passpine cli status                   # 查看 passpine 与 pass 的状态
```

链接默认放在 `~/.local/bin`，可用 `--dir` 指定其他目录。安装不会覆盖已有的 `pass` 或其他文件，卸载只移除指向当前 Passpine 的符号链接；若 PATH 中原版 pass 排在前面会给出提示。通过 `pass` 调用时，不带参数等同于 `pass ls`。

Windows 客户端以 GUI 子系统构建，暂不支持命令行：在终端中运行命令只会提示不支持并退出；从快捷方式等非终端位置带参数启动时仍打开桌面客户端。

开发时可用 `make client:install` 自动创建的 Dev 测试密码库验证命令行，详见[本地安装说明](install-local.md)。

## 支持的命令

| 命令 | 说明 |
| --- | --- |
| `ls` / `show` / 直接写条目名 | 列出目录树或输出条目；`-c[行号]` 复制指定行，`-q[行号]` 通过 `qrencode` 显示二维码 |
| `find`、`grep [-i] [-F]` | 按名称或解密内容查找；`grep` 使用 Go 正则语法 |
| `insert [-e \| -m] [-f]` | 新增条目，支持管道输入 |
| `edit` | 用 `$EDITOR`（默认 `vi`）编辑 |
| `generate [-n] [-c] [-q] [-i \| -f]` | 生成密码，默认 25 位 |
| `rm [-r] [-f]`、`mv [-f]`、`cp [-f]` | 删除、移动、复制条目或文件夹 |
| `otp [code] [-c]` | 输出 TOTP 验证码，兼容 pass-otp 的 `otpauth://` 行 |
| `git ...` | 在密码库目录中运行 Git |

与 pass 的差异：

- `init` 不可用。创建密码库、导入私钥均在客户端中完成，命令行沿用客户端保存的受口令保护私钥，不调用系统 GPG。
- `rm` 及 `mv -f` 覆盖的条目移入 Passpine 回收站，可在客户端中恢复。
- 每次命令独立解锁：已启用系统解锁时先请求系统验证，否则在终端输入私钥口令。客户端允许锁定时浏览名称时，`ls` / `find` 不需要解锁。
- 写入后若密码库是 Git 仓库，按 pass 的提交说明自动提交，只暂存 `.gpg` 和 `.gpg-id`，禁用 hooks；不会自动拉取或推送。

## 密码库与环境变量

默认使用客户端当前选择的密码库。`PASSWORD_STORE_DIR` 可选择其他已在客户端中创建或导入私钥的密码库。另支持：

- `PASSWORD_STORE_CLIP_TIME`：剪贴板清除秒数，未设置时使用客户端设置。
- `PASSWORD_STORE_GENERATED_LENGTH`：默认生成长度。
- `PASSWORD_STORE_X_SELECTION`：X11 / Wayland 使用的选区，默认 `clipboard`。
- `EDITOR`：`edit` 使用的编辑器。

复制使用 `wl-copy`、`xclip`、`xsel`（Linux）或 `pbcopy`（macOS）。后台进程到期后，只在剪贴板内容未被替换时清除。`edit` 优先在 `/dev/shm` 中写入临时明文；系统没有内存文件系统时会先提示确认，结束后覆盖并删除临时文件。命令行界面语言跟随客户端的语言设置。
