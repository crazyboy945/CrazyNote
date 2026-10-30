# CrazyNote Windows 版

Windows 上的完整可用版本，功能与 NAS 版一致：Markdown 存储、分类子目录、网页摘录。

---

## 一、快速开始

1. 解压 `CrazyNote-1.0.0.zip` 到任意目录（建议路径不含空格）
2. 双击 **`start.bat`**
3. 浏览器自动打开 `http://127.0.0.1:8899`
4. 首次运行会显示初始访问口令，复制输入即可登录

```
CrazyNote/
├── CrazyNote.exe              主程序（11 MB，无需安装运行时）
├── start.bat              一键启动（后台运行）
├── stop.bat               停止服务
├── install-autostart.bat  可选：注册开机自启
├── extension/             浏览器扩展（网页摘录用）
├── notes/                 ← 笔记存放目录，首次运行自动创建
├── appdata/               ← 配置目录（口令、会话密钥、令牌）
└── CrazyNote.log              运行日志
```

> **关于后台运行**：`start.bat` 用 `CrazyNote.exe -daemon` 启动服务，进程会完全脱离启动窗口，关掉黑窗口也不会中断服务。要停止服务，双击 `stop.bat`（或直接结束 `CrazyNote.exe` 进程）。

---

## 二、目录位置

| 内容 | 默认位置 | 说明 |
|------|----------|------|
| 笔记 | `<解压目录>\notes\` | 分类是其中的子目录，每篇笔记是一个 `.md` 文件 |
| 配置 | `<解压目录>\appdata\` | 口令哈希、会话密钥、摘录令牌 |
| 日志 | `<解压目录>\CrazyNote.log` | 启动信息、初始口令 |

**换位置**：改 `start.bat` 里的 `NOTES_DIR` / `APPDATA_DIR`，或直接用命令行参数：

```cmd
CrazyNote.exe -data "D:\我的笔记" -appdata "D:\crazynote-config" -addr 0.0.0.0:8899
```

想让它跟 Obsidian 等工具共存？把 `-data` 指向已有的库目录即可，元数据格式兼容。

---

## 三、局域网访问

默认监听 `0.0.0.0:8899`，同一 Wi-Fi 下的手机、平板、其他电脑都能访问：

```
http://<这台电脑的局域网IP>:8899
```

查看本机 IP：命令行执行 `ipconfig`，找「IPv4 地址」。

首次访问需要输入 `start.bat` 窗口里显示的初始口令。

> **安全提示**：口令保护是应用自带的，与 Windows 账户无关。若要暴露到公网，请务必用反向代理加 HTTPS，不要直接做端口映射。

---

## 四、开机自启（可选）

右键 `install-autostart.bat` → **以管理员身份运行**。

脚本会：
- 把程序安装到 `C:\ProgramData\CrazyNote`
- 注册名为 `CrazyNote` 的计划任务（开机以 SYSTEM 权限启动）
- 立即启动一次

访问地址仍是 `http://127.0.0.1:8899`，数据在 `C:\ProgramData\CrazyNote\notes`。

**卸载自启**：

```cmd
schtasks /Delete /TN CrazyNote /F
```

---

## 五、网页摘录（Windows 版）

功能与 NAS 版完全一致，两种方式：

### 浏览器扩展（推荐）

1. Chrome / Edge 打开 `chrome://extensions`（Edge 是 `edge://extensions`）
2. 右上角开启「开发者模式」
3. 点「加载已解压的扩展程序」，选择解压包里的 `extension` 目录
4. 点击扩展图标，填写：
   - **NAS 笔记服务地址**：`http://127.0.0.1:8899`
   - **摘录令牌**：在笔记网页右上角「⚙️ 设置」里点「生成」

之后点扩展图标选分类保存，或在页面右键选择「摘录此页到 CrazyNote」。

### 书签脚本

在笔记网页的「⚙️ 设置 → 书签脚本」里复制 `javascript:` 代码，拖到浏览器书签栏。点一下就把当前页面存进「摘录」分类。

---

## 六、命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-data` | 笔记根目录 | `<程序目录>\notes` |
| `-appdata` | 配置目录 | `<程序目录>\appdata` |
| `-addr` | 监听地址 | `:8899` |
| `-daemon` | 后台运行，脱离当前终端/窗口 | 关 |
| `-log` | 日志文件路径 | 无（输出到 stdout） |
| `-version` | 打印版本 | — |

环境变量 `CRAZYNOTE_DATA_DIR` / `CRAZYNOTE_APPDATA` / `CRAZYNOTE_ADDR` 同等生效。

手动后台启动示例：

```cmd
CrazyNote.exe -daemon -log CrazyNote.log -data "D:\我的笔记" -appdata "D:\crazynote-config" -addr 0.0.0.0:8899
```

---

## 七、常见问题

**Q：双击 start.bat 一闪而过？**
A：端口 8899 可能被占用。改 `start.bat` 里的 `PORT` 值，或用命令行运行查看报错。

**Q：想开机自启但提示权限不足？**
A：`install-autostart.bat` 必须右键「以管理员身份运行」。

**Q：能多开一个实例吗？**
A：可以，用不同的 `-addr` 和 `-data`：

```cmd
CrazyNote.exe -addr 127.0.0.1:8900 -data "D:\笔记2" -appdata "D:\配置2"
```

**Q：怎么彻底卸载？**
A：删掉整个解压目录即可。若注册过自启，先执行 `schtasks /Delete /TN CrazyNote /F`。

**Q：忘记口令了？**
A：删掉 `appdata\credentials.json`，重启后会生成新口令（旧会话失效，但笔记不受影响）。

**Q：防火墙弹窗？**
A：首次启动 Windows 会询问网络权限，选「允许」才能让局域网其他设备访问。也可以手动放行：

```cmd
netsh advfirewall firewall add rule name="CrazyNote" dir=in action=allow protocol=TCP localport=8899
```
