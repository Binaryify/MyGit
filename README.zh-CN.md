<p align="center">
  <img src="docs/images/logo.png" width="128" height="128" alt="MyGit">
</p>

<h1 align="center">MyGit</h1>

<p align="center">
  轻量、快速的原生界面 git 客户端。<br>
  JetBrains 的分支图和历史操作，VS Code 的暂存方式，内存却只要它们的零头。
</p>

<p align="center">
  <a href="https://github.com/Binaryify/MyGit/actions/workflows/ci.yml"><img src="https://github.com/Binaryify/MyGit/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/macOS-Apple%20芯片-111?logo=apple" alt="macOS">
  <img src="https://img.shields.io/badge/Windows-x64-0078D4?logo=windows" alt="Windows">
  <img src="https://img.shields.io/badge/Linux-x64-FCC624?logo=linux&logoColor=111" alt="Linux">
  <img src="https://img.shields.io/badge/built%20with-Go%20%2B%20MyGo-00ADD8?logo=go&logoColor=white" alt="Go">
</p>

<p align="center">
  <a href="README.md">English</a> · <b>简体中文</b>
</p>

<p align="center">
  <img src="docs/images/main-zh.png" alt="MyGit 显示仓库历史和提交的差异">
</p>

<p align="center">
  <a href="site/assets/mygit-demo-zh.mp4"><b>▶ 观看 40 秒演示视频</b></a>
</p>

## 为什么是 MyGit

基于 IDE 平台的独立 git 客户端，会把整个 IDE 一起带上。MyGit 是一个原生二进制文件，由 [MyGo](https://github.com/egoist/mygo) 用 GPU 绘制界面：没有 webview，也没有 JVM。

打开一个 2 万个提交的仓库后的内存占用，应用的所有进程都计算在内（Apple M4）：

| 应用 | 内存 |
|---|---|
| **MyGit** | **109 MB** |
| 精简版 IntelliJ 客户端 | 750 MB |
| VS Code，不带插件 | 1000 MB |
| VS Code，167 个插件 | 1400 MB |
| IntelliJ IDEA 2026.2 | 2300 MB |

在同一个仓库上，MyGit 读完全部历史并布局分支图只要 **0.13 秒**，打开一个 20 MB 补丁的提交（5677 个文件）只要 **0.24 秒**，空闲时 CPU 为 **0%**。

MyGit 读取靠解析 `git` 的输出，所有改动都交给 `git` 命令本身完成，所以仓库在 MyGit 里的表现和在终端里完全一致。

## 为什么不用 Sourcetree？

Sourcetree 免费、成熟，但它只是一个 git 客户端。MyGit 是为 AI 写代码的时代做的：改动交给 AI，你不用打开 IDE，就能看清它改了什么，审阅、顺手修一行、暂存、提交。除此之外：

| | MyGit | Sourcetree |
|---|---|---|
| Linux | **支持** | 不支持，只有 macOS 和 Windows |
| 各平台同一个应用 | **是**，同一套代码、同样的功能 | macOS 和 Windows 是两个不同的应用，功能有差异 |
| 解决冲突 | **内置**：我们的、结果、他们的三栏并排，一个冲突一个冲突地处理 | 整个文件采用某一边，或交给外部合并工具 |
| 暂存前直接在 diff 里改一行 | **可以** | 不行 |

## 功能

<table>
  <tr>
    <td width="50%"><img src="docs/images/changes-zh.png" alt="在 diff 里编辑改动"></td>
    <td width="50%"><img src="docs/images/merge-zh.png" alt="解决冲突"></td>
  </tr>
  <tr>
    <td><b>想暂存什么就暂存什么。</b>按文件、按改动块、按行暂存，还能直接在 diff 里编辑工作区文件。</td>
    <td><b>三栏解决冲突。</b>我们的、结果、他们的，一个冲突一个冲突地处理。</td>
  </tr>
  <tr>
    <td><img src="docs/images/branches-zh.png" alt="搜索分支"></td>
    <td><img src="docs/images/welcome-zh.png" alt="欢迎窗口"></td>
  </tr>
  <tr>
    <td><b>任何分支都能找到。</b>搜索本地和远程分支，切换、合并、变基或比较。</td>
    <td><b>怎么打开都行。</b>拖入文件夹、克隆，或者从最近打开里选。</td>
  </tr>
</table>

- **历史**：所有分支、标签、远程，带分支图，边读边显示；按说明、作者、哈希搜索；按分支或路径筛选（文件历史）
- **提交**：完整说明、改动文件、带语法高亮的并排 diff；合并提交可以选对比哪个父提交；比较两个提交（⌘ 点选）、分支或工作区
- **暂存**：按文件、改动块或选中的行暂存、取消暂存、丢弃；在 diff 里编辑并保存工作区文件
- **历史操作**：新建分支和标签、检出、cherry-pick、revert、reset（soft / mixed / hard）、交互式变基（重排、改写说明、停下修改、squash、fixup、丢弃）
- **审阅**：生成的文件（锁文件、压缩文件、`linguist-generated`）默认折叠；可以把文件标记为已看；图片改动显示前后对比；可以评论某几行，并把评论复制为 Markdown 交给 AI 或同事
- **拉取请求**：打开 GitHub 拉取请求，在对应行下阅读和回复讨论、解决讨论，并提交审阅：评论、批准或请求修改
- **冲突**：三栏合并视图；合并、变基、cherry-pick、revert 中途可以继续、跳过或中止
- **Stash、Blame 和远程**：stash（可包括未跟踪文件）、逐行追溯、远程管理、拉取（快进 / 变基 / 合并）、推送前预览、force-with-lease
- **登录**：git 需要用户名、令牌或口令时，在窗口里询问
- **随处可用**：每个仓库一个窗口、拖入文件夹打开、终端里 `mygit .`、自定义标题栏、深浅色、English 和简体中文、签名的自动更新

## 安装

Release 即将发布。在那之前，可以用 [Go](https://go.dev/dl/) 1.27 或更新版本从源码构建：

```bash
git clone https://github.com/Binaryify/MyGit.git
cd MyGit
go tool mygo build    # 生成 build/darwin-arm64/MyGit.app 和磁盘镜像
```

MyGit 需要 `PATH` 里有 `git`。支持 macOS（Apple 芯片）、Windows（x64）和 Linux（x64），不支持 Intel Mac。

## 快捷键

| 按键 | 作用 |
|---|---|
| ⌘↩ / Ctrl+Enter | 提交 |
| ⌘F / Ctrl+F | 搜索历史 |
| ⌘R / Ctrl+R | 刷新 |
| ⌘P / Ctrl+P | 转到提交或更改里的文件 |
| ⌘K / Ctrl+K | 命令栏 |
| ⌘S / Ctrl+S | 保存在 diff 里编辑的文件 |
| 空格 | 暂存或取消暂存选中的改动；把提交里的文件标记为已看 |
| Delete | 丢弃选中的改动 |
| ⌘ 点选 | 比较两个提交 |

## 开发

```bash
go tool mygo dev      # 运行，改动后自动重新加载
go test ./...         # 测试，界面测试也不需要窗口
go tool mygo build -platform darwin/arm64,windows/amd64,linux/amd64
```

界面文字在 [`locales/`](locales) 里：`en.json` 是源语言，`zh-Hans.json` 是简体中文，改完运行 `go tool mygo generate`。欢迎贡献其他语言的翻译：新增 `locales/<语言标签>.json` 即可。

发布：在 [`CHANGELOG.md`](CHANGELOG.md) 写好这个版本的说明，再推送 `v0.2.0` 这样的标签。CI 会用仓库 Secret `MYGO_UPDATER_PRIVATE_KEY` 给更新包签名，打包 macOS 磁盘镜像、Windows 安装程序和 zip、Linux 安装包，并创建 Release。签名私钥在本机 `~/Library/Application Support/mygo/update-keys/mygit/`，丢失后已安装的应用就收不到更新了。仓库是私有的时候，已安装的应用下载不到 Release 里的更新包，测试自动更新前要先把仓库公开，或者把更新包放到公开地址。

基准测试：`MYGIT_BENCH_REPO=<仓库> MYGIT_BENCH_COMMIT=<提交> go test -run '^$' -bench .`
