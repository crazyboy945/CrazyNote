# CrazyNote — 飞牛 NAS 上的 Markdown 笔记应用

在飞牛 fnOS 上运行的单文件二进制笔记服务。笔记以纯 `.md` 文件存储在 NAS 硬盘上，**每个分类对应一个子目录**，任何编辑器、Obsidian、Typora、VS Code 都能直接打开。支持网页摘录。

---

## 一、核心特性

| 特性 | 说明 |
|------|------|
| 纯 Markdown 存储 | 每篇笔记就是一个 `.md` 文件，YAML frontmatter 保存元数据，无数据库、无锁定文件 |
| 分类 = 子目录 | 在界面里建分类，硬盘上就是同名文件夹；SMB / WebDAV / 文件管理器里直接可见 |
| 文件名跟随标题 | 改标题会自动重命名文件，目录里一眼能认出内容 |
| 网页摘录 | Chrome/Edge 扩展（Readability 抽正文）+ 通用书签脚本，一键存为 Markdown |
| 代码语法高亮 | 服务端 chroma 渲染，支持 200+ 语言，亮/暗色主题自动切换；网页版与桌面端一致 |
| fnOS 账号免登 | 走 fnOS 统一网关，直接用 NAS 账号打开桌面图标即用，无需再输口令 |
| 局域网直连 | 同时监听 TCP 端口，可用应用自身的口令访问 |
| 桌面客户端 | Windows 原生程序（Electron），连接 NAS 服务端浏览编辑，支持托盘常驻与剪贴板速记 |
| 单文件部署 | 静态链接二进制（7~8MB），不依赖 Docker、不依赖运行时 |
| 内容可移植 | 整个笔记目录拷走即可迁移，或直接纳入 Git / Syncthing 同步 |
| 图片上传与插入 | 编辑器内一键上传本地图片，自动以 `![alt](url)` 插入光标处，图片存于隐藏目录 `.crazynote/assets/images/` |
| 阅读/编辑一体 | 默认阅读态（渲染富文本、代码块高亮），点击正文即进入编辑（Markdown 源文） |
| Markdown 样式切换 | 内置简约 / 悦读 / 学术 / 卡片 / 紧凑 5 套阅读样式，设置里一键切换并自动保存 |
| 备份与恢复 | 设置里一键导出全量备份 ZIP（分类 / 笔记 / 图片 / 设置），恢复前自动快照，出错可回退 |

---

## 二、编辑器：阅读与编辑视图

CrazyNote 把「阅读」和「编辑」放在同一个界面里，**默认阅读、点击即编辑**：

- **默认阅读态**：打开任意笔记直接看到渲染后的富文本，标题、代码块（带语法高亮）、表格、图片都按最终效果呈现。
- **点击即编辑**：在阅读视图里点击正文任意位置，立即切换到编辑态（Markdown 源文文本框 + 标题/标签输入）。改完点顶栏「👁 预览」或再次点击正文即可回到阅读。
- **图片一键插入**：点工具栏「🖼️ 图片」选择本地图片，上传后自动在光标处插入 `![图片名](/assets/images/xxx.png)`，切回阅读即见。
- **链接不误触**：点击正文里的超链接会正常跳转，不会因此切到编辑态。

> 图片统一存放在笔记根目录下的隐藏目录 `.crazynote/assets/images/`，不会污染分类目录、也不会被当成 `.md` 笔记扫描；文件名会做安全化处理，并拒绝路径穿越。
>
> 笔记里记录的图片地址是稳定的 `/assets/images/xxx.png`，**与访问方式无关**（飞牛桌面图标、局域网直连都指向同一个地址），
> 网关前缀由服务端在渲染时按需补上。这样同一篇笔记换用别的工具打开、或改用局域网直连访问时，图片都不会失效。

### Markdown 样式

内置 5 套阅读样式，在「⚙️ 设置 → Markdown 样式」里**点击即切换**，自动保存到 `<笔记目录>/.crazynote/meta.json`，对所有笔记生效，亮色 / 暗色模式都适配。

| 样式 | 特点 | 适合 |
|------|------|------|
| 简约 | 无衬线、适中行距，界面默认风格 | 日常通用 |
| 悦读 | 衬线字体、宽行距、限宽居中、标题主色 | 长文阅读 |
| 学术 | 衬线字体、两端对齐、首行缩进 2 字、标题居中 | 论文 / 文档 |
| 卡片 | 圆角卡片 + 轻阴影、大留白 | 长文、截图说明 |
| 紧凑 | 小字号、紧行距、压缩段落间距 | 技术笔记、密集信息 |

> 样式只影响**阅读视图**的渲染效果，不改变 `.md` 源文件内容，用其它工具打开时完全不受影响。

## 三、目录结构

```
/你的笔记目录/                 ← 存储位置（安装向导可指定，默认在应用数据目录）
├── .crazynote/
│   ├── meta.json             ← 分类排序等应用元数据
│   └── trash/                ← 已删除的分类（可恢复）
├── 技术/                      ← 一个分类 = 一个子目录
│   ├── Go 交叉编译笔记.md
│   └── 深入理解 Go 的并发模型.md
├── 读书/
└── 摘录/                      ← 网页摘录默认落点
```

单篇笔记的实际格式：

```markdown
---
id: ba73b66f73d822d1
title: 深入理解 Go 的并发模型
created: "2026-10-27T09:45:10+08:00"
updated: "2026-10-27T09:45:10+08:00"
category: 摘录
source: https://blog.example.com/go-concurrency
---

来源: [深入理解 Go 的并发模型](https://blog.example.com/go-concurrency)

# 深入理解 Go 的并发模型

正文……
```

> 正文是标准 Markdown，表格、代码块、引用、任务列表都能正常渲染。用其他工具打开同一个目录也完全没问题。

---

## 四、部署方式

### 方式 A：FPK 应用包（推荐，桌面图标入口）

**打包**（在有 Go 环境的机器上执行）：

```bash
bash scripts/build.sh          # 交叉编译 x86_64 / arm64 / armv7l
python scripts/build_fpk.py    # 生成 dist/crazynote-1.0.8.fpk
```

> **版本号（重要）**：fnOS 应用中心按 manifest 里的 `version` 判断能否覆盖安装，
> **版本号不更新会被判成「已安装更高版本」而拒绝安装**。
> 打包时用 `VERSION` 环境变量统一指定，脚本会自动把它写进 manifest 与二进制：
>
> ```bash
> VERSION=1.0.4 bash scripts/build.sh
> VERSION=1.0.4 python scripts/build_fpk.py
> ```
>
> 还可用 `CHANGELOG="本次更新说明"` 覆盖 manifest 的 `changelog`（升级时展示给用户）。
> `manifest` 里的 `version` 只是兜底值，每次打包都会被 `VERSION` 覆盖。

> **架构说明**：FPK 包的 `manifest` 里用 `platform` 字段声明架构（`x86` / `arm` / `all`），
> 打包脚本会按该字段**只打包对应架构的二进制**。默认是 `x86`。
> 如果你的 NAS 是 ARM 架构，把 manifest 里的 `platform=x86` 改成 `platform=arm` 再打包。
> 同时 `os_min_version` 声明最低系统版本（默认 `0.9.0`）。

**安装**：

1. 把 `dist/crazynote-1.0.8.fpk` 传到 NAS（可走文件管理或 SMB）
2. 飞牛桌面 → 应用中心 → 手动安装 → 选择该 fpk
3. 向导里填写**笔记存储目录**（例如 `/vol1/1000/Notes`）和局域网端口
4. 装完后飞牛桌面出现「CrazyNote」图标，点击即用（走统一网关，NAS 账号免登）

> **系统版本**：手动安装需要 fnOS 0.9.0 及以上版本（manifest 中 `os_min_version=0.9.0` 已按此设置）。

也可以用命令行安装：

```bash
appcenter-cli install-fpk crazynote-1.0.8.fpk
```

### 方式 B：裸二进制（SSH 直接跑）

```bash
# 1. 上传对应架构的二进制到 NAS
#    x86_64 NAS 用 crazynote-linux-amd64，ARM NAS 用 crazynote-linux-arm64

# 2. 创建笔记目录
mkdir -p /vol1/1000/Notes

# 3. 启动（前台）
./crazynote-linux-amd64 \
  -data /vol1/1000/Notes \
  -appdata /var/apps/crazynote/etc \
  -addr 0.0.0.0:8899

# 4. 后台常驻
nohup ./crazynote-linux-amd64 \
  -data /vol1/1000/Notes \
  -appdata /var/apps/crazynote/etc \
  -addr 0.0.0.0:8899 \
  >> /var/log/crazynote.log 2>&1 &

# 5. 自启动（systemd）
cat > /etc/systemd/system/crazynote.service <<'EOF'
[Unit]
Description=CrazyNote - Markdown 笔记服务
After=network.target

[Service]
Type=simple
ExecStart=/opt/crazynote/crazynote -data /vol1/1000/Notes -appdata /var/apps/crazynote/etc -addr 0.0.0.0:8899
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload && systemctl enable --now crazynote
```

启动后访问 `http://<NAS内网IP>:8899`，首次启动日志里会打印初始访问口令。

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-data` | 笔记根目录 | `$CRAZYNOTE_DATA_DIR` → `$TRIM_PKGVAR/notes` → `<appdata>/notes` |
| `-appdata` | 配置目录（口令哈希、会话密钥、令牌） | `$TRIM_PKGETC/crazynote` → `~/.config/crazynote` |
| `-addr` | 局域网监听地址 | `:8899` |
| `-socket` | fnOS 统一网关 Unix Socket 路径 | 空（不启用网关模式） |
| `-prefix` | 网关路径前缀 | `/app/crazynote` |
| `-version` | 打印版本 | — |

---

## 五、网页摘录

### 方式 1：浏览器扩展（Chrome / Edge）

扩展目录：`extension/`

1. 打开 `chrome://extensions`（Edge 是 `edge://extensions`）
2. 右上角打开「开发者模式」
3. 点「加载已解压的扩展程序」，选择本项目的 `extension` 目录
4. 点击扩展图标，填写：
   - **NAS 笔记服务地址**：如 `http://192.168.1.10:8899`（FPK 方式用 `https://你的NAS域名/app/crazynote`）
   - **摘录令牌**：在笔记应用的「⚙️ 设置」里点「生成」得到
5. 之后：
   - 点扩展图标 → 选分类 → 保存（自动提取正文，去广告去导航）
   - 页面右键 → 「摘录此页到 CrazyNote」/「摘录选中内容到 CrazyNote」

> 扩展内置 Mozilla Readability（Apache-2.0）做正文识别，服务端再兜一层降噪：脚本、样式、导航栏、侧栏推荐、评论区、页脚全部剔除，表格转成 Markdown 表格，代码块保留。

### 方式 2：书签脚本（任意浏览器 / 手机）

在笔记应用的「⚙️ 设置 → 书签脚本」里会生成一段 `javascript:` 代码，**拖到浏览器书签栏**即可。

点击书签会把当前页面正文存进「摘录」分类。手机上用 Safari/Chrome 添加书签同样可用。

### 服务端接口

```
POST /api/clip
{
  "category": "摘录",          // 目标分类，不存在会自动创建
  "title": "文章标题",
  "url": "https://example.com/post",
  "html": "<html>...</html>",  // 或 "markdown" / "body" / "selection"
  "selection": "选中的文字",    // 优先级最高，只存这一段
  "tags": ["参考"]
}
```

认证方式三选一：`X-CrazyNote-Token` 头、`?token=` 查询参数、或登录后的会话 Cookie。

---

## 六、代码语法高亮

服务端用 [chroma](https://github.com/alecthomas/chroma) 做语法高亮，网页版与桌面端共用同一套渲染逻辑，**200+ 种语言**开箱即用。

### 写法

标准 Markdown 围栏代码块，指定语言即可：

````markdown
```go
package main

import "fmt"

func main() {
	fmt.Println("Hello")
}
```
````

语言名不区分大小写（`Go`、`GO`、`go` 等价）。未指定语言时按纯文本显示。常见别名都支持：`js`/`javascript`、`py`/`python`、`ts`/`typescript`、`sh`/`bash`/`shell`、`yml`/`yaml`、`html`/`xml`、`c`/`cpp`/`csharp` 等。

### 高亮主题

跟随系统亮/暗色自动切换：

| 模式 | 主题 | 说明 |
|------|------|------|
| 亮色 | `github` | GitHub 经典配色 |
| 暗色 | `github-dark` | GitHub 暗色配色 |

配色规则由服务端 `/api/highlight.css` 提供（也可访问 `/assets/highlight-light.css`、`/assets/highlight-dark.css` 直接取静态文件）。

### 接口

```
POST /api/render
{
  "markdown": "```go\nfunc main() {}\n```"
}
→ { "html": "<pre class=\"cnchroma\">..." }
```

预览时前端调用此接口拿 HTML，输入有 180ms 防抖，不会因每次按键都发请求。

---

## 七、桌面客户端

Electron 桌面程序，连接 NAS 上的 CrazyNote 服务端浏览与编辑笔记。**Windows 与 Linux 双平台**。

### 使用

| 平台 | 产物 | 安装方式 |
|------|------|----------|
| Windows | `CrazyNote-1.0.1-x64.exe`（安装包）、`CrazyNote-1.0.1-x64.zip`（免安装） | 双击安装，或解压后直接运行 |
| Linux | `CrazyNote-1.0.1-linux-x64.AppImage`（单文件）、`CrazyNote-1.0.1-linux-x64.deb`（Debian/Ubuntu）、`CrazyNote-1.0.1-linux-x64.tar.gz`（免安装） | AppImage 加执行权限直接运行；deb 用 `sudo apt install ./xxx.deb` |

启动后填服务端地址与访问口令 → 连接。

| 功能 | 说明 |
|------|------|
| 三栏界面 | 左侧分类与标签、中间笔记列表、右侧编辑与预览 |
| 全文搜索 | `Ctrl+F` 聚焦搜索框 |
| 快捷键 | `Ctrl+N` 新建、`Ctrl+S` 保存、`Ctrl+Shift+V` 摘录剪贴板、`Ctrl+,` 设置 |
| 系统托盘 | 常驻后台，右键菜单可新建笔记、摘录剪贴板、退出 |
| 剪贴板速记 | `Ctrl+Shift+V` 把剪贴板内容直接存进「摘录」分类 |
| 凭据安全 | 口令用系统 `safeStorage` 加密存储；Linux 无系统钥匙环时自动降级为本地编码 |
| 亮暗适配 | 跟随系统主题，代码高亮同步切换 |

> **Linux 说明**：
> - 免安装包解压后自带 `install.sh`：执行 `./install.sh` 装到用户目录（`~/.local`）并自动创建应用菜单入口与图标；
>   `./install.sh --system` 需要 root（可正确设置 `chrome-sandbox` 的 setuid，保留完整沙箱保护）；
>   `./install.sh --uninstall` 卸载。
> - 托盘图标在 GNOME 下需要 AppIndicator 扩展支持（Ubuntu 需安装 `gnome-shell-extension-appindicator`）；
>   KDE / XFCE 等原生支持托盘的桌面环境开箱即用。
> - 无钥匙环（gnome-keyring / kwallet）时口令以本地编码保存，安全性略降，建议在 NAS 端使用强口令。
> - 依赖：Debian/Ubuntu 需 `libgtk-3-0`、`libnotify4`、`libnss3` 等运行时库（deb 包已声明依赖）。
> - AppImage 与 deb 必须在 Linux 宿主上打包（分别依赖 `mksquashfs` 与 Ruby `fpm`），
>   仓库内置 GitHub Actions 工作流 `.github/workflows/build-desktop.yml`，推送后自动在 `ubuntu-latest` 上构建。

### 开发

```bash
cd desktop
npm install            # 首次需下载 Electron（约 100MB）
npm start              # 开发模式
npm run icons          # 重新生成图标（含 Linux 多尺寸图标集）
npm run dist           # 打包 Windows 安装包与免安装 zip
npm run dist:linux     # 打包 Linux（AppImage + deb + tar.gz）
npm run dist:linux:dir # 仅解包 Linux 目录，便于调试
```

架构：主进程负责所有 HTTP 请求（规避浏览器 CORS 限制）与会话 Cookie 管理，渲染层只通过 `contextBridge` 暴露的受限接口通信。

> **注意**：本机若设置了 `ELECTRON_RUN_AS_NODE=1`，Electron 会以纯 Node 模式启动导致 `app is undefined`，启动前需取消该环境变量。

---

## 八、认证说明

| 场景 | 认证方式 |
|------|----------|
| fnOS 桌面图标（统一网关） | 网关校验 NAS 登录态后注入 `X-Trim-Username` / `X-Trim-Isadmin`，应用自动识别，**免登** |
| 局域网直连 `http://IP:8899` | 应用自身的访问口令（首次启动日志打印，可在设置里修改） |
| 扩展 / 书签脚本 | 摘录令牌（API Token） |

安全设计：只有 Unix Socket 监听才信任网关注入的身份头；TCP 端口上任何客户端自带的 `X-Trim-*` 头都会被忽略。路径穿越、非法文件名、非法分类名一律拒绝。

---

## 九、备份与迁移

### 应用内备份与恢复

「⚙️ 设置 → 备份与恢复」提供两个按钮：

| 操作 | 说明 |
|------|------|
| **导出备份** | 把整个笔记目录打包成 ZIP 下载，含**全部分类、笔记、样式设置与上传的图片** |
| **恢复备份** | 选择备份 ZIP 覆盖恢复；恢复前系统会**自动把现有内容快照**到 `<笔记目录>/.crazynote/restore-backups/<时间戳>/` |

恢复是破坏性操作，因此：

- 必须先在弹窗里确认，服务端还要求请求带确认标记，避免误触；
- 压缩包会先做**完整校验**（拒绝 `..` 路径穿越、绝对路径、超大文件与 zip 炸弹），**校验不通过直接拒绝、绝不改动现有数据**；
- 校验通过后先快照再解压，出问题可以从 `.crazynote/restore-backups/` 手工找回；
- 历史快照目录**不会**被打进下一次备份，避免备份文件无限膨胀。

### 手工备份

- **备份**：复制整个笔记目录即可（含 `.crazynote/`）。目录里没有数据库文件，不需要停服。
- **迁移**：把目录拷到另一台 NAS 或本地，改 `-data` 指向它。
- **同步**：目录结构简单、无锁文件，可以直接用 Syncthing / 飞牛同步 / Git 管理。
- **与 Obsidian 等工具共存**：直接把笔记目录作为 Obsidian 库打开即可，元数据格式兼容。

---

## 十、开发与测试

```bash
# 单元测试（存储、摘录解析、daemon 派生）
go test ./...

# 接口端到端自测（需先启动服务）
bash scripts/build.sh
./crazynote -data /tmp/notes -appdata /tmp/appdata -addr 127.0.0.1:18899 &
bash test/api_test.sh

# 指定服务地址、笔记目录、日志位置（跨平台通用）
CRAZYNOTE_BASE="http://127.0.0.1:8899" \
CRAZYNOTE_NOTES="/path/to/notes" \
CRAZYNOTE_LOG="/path/to/server.log" \
CRAZYNOTE_WORK="/tmp/crazynote-test" \
bash test/api_test.sh

# Windows 上的浏览器 UI 实测（需已安装 playwright）
python test/ui_test.py
```

接口自测覆盖 42 项：目录结构落盘、frontmatter、搜索、摘录降噪、Token 鉴权、路径穿越防护、分类增删改、文件重命名、跨分类移动等。两个测试脚本均为幂等，可重复执行。

### 目录结构

```
CrazyNote/
├── cmd/crazynote/main.go         入口，参数解析
├── internal/
│   ├── model/note.go              笔记模型、frontmatter 读写、文件名安全处理
│   ├── store/store.go             磁盘存储：分类目录、笔记读写、搜索
│   ├── clip/clip.go               网页正文提取 + HTML→Markdown
│   ├── render/render.go           Markdown 渲染（服务端预览）
│   ├── auth/auth.go               网关身份识别、口令、会话、API Token
│   ├── config/config.go           配置解析（命令行 > 环境变量 > 向导文件）
│   └── server/                    HTTP 路由、静态资源内嵌
│       └── web/                   前端（原生 JS，无构建步骤）
├── extension/                     Chrome/Edge MV3 扩展
├── packaging/crazynote/               FPK 打包骨架（manifest / cmd / wizard / config）
├── scripts/build.sh               交叉编译
├── scripts/build_fpk.py           FPK 组装
└── test/api_test.sh               接口端到端自测
```

技术栈：Go 标准库 + `goldmark`（Markdown 渲染）、`chroma`（代码语法高亮）、`html-to-markdown`（HTML 转 Markdown）、`yaml.v3`（frontmatter）。前端是原生 HTML/CSS/JS，无构建步骤，全部内嵌进二进制。

---

## 十一、常见问题

**Q：飞牛桌面的图标打不开 / 白屏？**
A：检查 `config/privilege` 的运行用户是否有权限访问 `${TRIM_APPDEST}`，以及网关前缀 `/app/crazynote` 与 `-prefix` 是否一致。

**Q：局域网访问提示未认证？**
A：局域网直连用的是应用口令，不是 NAS 账号密码。首次启动的日志里有初始口令，或在设置里重设。

**Q：扩展提示「无法连接 NAS」？**
A：检查地址是否含 `http://`、令牌是否正确。FPK 部署要用 `https://你的NAS域名/app/crazynote` 这种网关地址。

**Q：笔记文件名带奇怪字符？**
A：文件名会自动做安全化处理（去掉 `/ \ : * ? < > |` 等非法字符），中文标题正常保留。

**Q：删除的分类能恢复吗？**
A：能。被删的分类会移到 `<笔记目录>/.crazynote/trash/` 下，带时间戳，手动移回即可。

**Q：启动报错 `mkdir .../.crazynote/meta.json: not a directory`？**
A：这是 v1.0.4 及更早版本的缺陷（早期把 `meta.json` 误建成目录，之后保存设置时又被改写为文件，于是二次启动就失败）。
**v1.0.5 已修复**，升级后即可正常启动；若暂时无法升级，可手动删掉它再重启：

```bash
rm -rf /usr/local/apps/@appdata/crazynote/notes/.crazynote/meta.json
```

> `.crazynote/` 目录本身要保留（存分类排序、样式偏好等元数据）；删掉只丢设置，不影响笔记正文。

**Q：怎么确认 NAS 上装的是哪个版本？**
A：打开「⚙️ 设置」，标题下方会直接显示当前版本号（如「版本 1.0.8」）。命令行执行 `crazynote -version` 也能看到。

**Q：Linux 桌面版托盘没有图标？**
A：GNOME 需安装 `gnome-shell-extension-appindicator`；此外若托盘创建失败，应用会直接退出，可改用 `--no-sandbox` 手动启动查看报错。
