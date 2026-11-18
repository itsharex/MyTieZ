# MyTieZ

基于 Go 与原生 UI 框架 [MyGo](https://github.com/egoist/mygo) 重写的轻量级桌面剪贴板管理工具，界面设计与交互灵感源自 [tiez-clipboard](https://github.com/jimuzhe/tiez-clipboard)。

## 功能特性

- **多类型剪贴板历史**：支持纯文本、富文本（HTML 预览）、链接（URL）、代码片段、图片（缩略图预览）与文件列表，内置启发式链接/代码识别。
- **快速检索与筛选**：支持关键字搜索、`#标签` 过滤、6 大类型筛选胶囊与自定义标签管理。
- **快捷操作与粘贴**：
  - 全局快捷键呼出（支持自定义热键录制，默认 `Alt+V`）；
  - 鼠标跟随弹出（靠近屏幕底部自动向上翻转）或居中弹出，支持窗口置顶与失焦自动隐藏；
  - **不抢焦点模式（实验性）**：呼出、点击与拖拽面板时不抢占当前前台窗口焦点，并支持点击窗口外自动隐藏（在搜索框或标签输入时自动临时恢复输入焦点）；
  - 支持数字键快捷粘贴（`Alt+1~9` / `Ctrl+1~9`）、粘贴后自动删除、点击仅复制、外部应用打开。
- **外观与本地化**：
  - 支持浅色 / 深色 / 跟随系统主题，以及 Acrylic / Mica 等背景材质切换；
  - 内置简体中文、繁體中文、English 三语切换。
- **本地存储**：使用纯 Go SQLite（`modernc.org/sqlite`，`CGO_ENABLED=0`）本地持久化，图片文件按需缩略加载。

## 平台支持说明

- **Windows（主力支持）**：完整支持剪贴板监听、来源应用与图标捕获、前台窗口追踪、自动模拟粘贴（`Ctrl+V`）、UAC 提权重启及系统托盘。
- **macOS**：基于 MyGo 的 UI、剪贴板监听/读写、搜索、标签与托盘均可直接构建运行；但 `internal/sys` 中自动粘贴（`Cmd+V`）、来源应用捕获与前台窗口追踪目前为跨平台占位实现，点击条目仅写入剪贴板，尚未补齐 macOS 原生系统桥接。

## 开发与测试

环境要求：Go `1.27+`，并安装 [MyGo CLI](https://github.com/egoist/mygo)：

```powershell
# 安装 mygo CLI
go install github.com/egoist/mygo/cmd/mygo@latest

# 开发预览
mygo dev

# 修改 locales/*.json 后重新生成多语言目录
mygo generate

# 运行单元测试
go test -count=1 ./internal/...

# 静态检查
mygo vet .

# 打包发布
mygo build
```

## 目录结构

```text
├── main.go              # 应用入口与内嵌资源
├── mygo.json            # MyGo 应用与 i18n 配置
├── locales/             # 简体中文 / 繁體中文 / English 语言文件与生成代码
└── internal/
    ├── app/             # 主面板、设置页、标签浮层、热键录制与缓存
    ├── store/           # SQLite 存储、内容分类与配置持久化
    └── sys/             # 平台原生能力（模拟粘贴、来源应用、窗口焦点、内存修剪）
```

## 致谢

- 界面设计与核心交互参考自 [jimuzhe/tiez-clipboard](https://github.com/jimuzhe/tiez-clipboard)，在此基础上使用 MyGo 原生 UI 重新实现。
- 感谢 [egoist/mygo](https://github.com/egoist/mygo) 提供的 Go 原生桌面 UI 与系统集成框架。

## License

[GPL-3.0](./LICENSE)
