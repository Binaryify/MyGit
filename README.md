# MyGit

轻量的 git 客户端，用 [MyGo](https://github.com/egoist/mygo) 原生 UI 写成，交互参考 IntelliJ / Rebased 的 git 工具。

- **历史**：所有分支、标签、远程，带分支图，流式加载；按说明/作者/哈希搜索，按分支或路径筛选（文件历史）
- **提交详情**：完整说明、改动文件、并排 diff（语法高亮），合并提交可切换对比的父提交
- **暂存**：按文件、按改动块、按选中行暂存、取消暂存、丢弃（丢弃会先确认）
- **提交**：提交、修改上次提交；文件变化时自动刷新
- **分支**：新建、切换、重命名、删除（未合并时确认），检出、合并、删除远程分支，合并、变基
- **提交右键菜单**：复制哈希、新建分支或标签、检出、cherry-pick、revert、reset（soft/mixed/hard）、交互式变基
- **交互式变基**：拖动重排，pick / reword / edit / squash / fixup / drop
- **冲突**：三栏合并视图（我们的 | 结果 | 他们的），按冲突块采用某一边或两边都要；合并、变基、cherry-pick 中途的继续、跳过、中止
- **Stash**：stash（可包括未跟踪文件）、查看、应用、弹出、从 stash 建分支、删除
- **Blame**：逐行追溯，双击定位到对应提交
- **远程**：获取；拉取（快进、变基、合并）；推送前预览提交，支持 force-with-lease 和推送标签
- **多仓库**：每个仓库一个窗口，最近打开，命令行再次启动时交给已运行的实例（`mygit .`）
- **界面**：自定义标题栏，跟随系统深浅色；中文 / English，跟随系统或在设置里切换
- **更新**：启动后和每 6 小时检查一次新版本，签名校验后一键更新
- 读取靠解析 `git` 的输出，写操作全部交给 `git` 命令本身，不会弹出编辑器

支持 macOS（Apple 芯片）、Windows（x64）、Linux（x64）；不支持 Intel Mac。

```bash
go run . ~/path/to/repo
```

## 性能（Apple M4，2 万个提交的仓库）

| | MyGit | Rebased |
|---|---|---|
| 打开后内存（footprint） | 109 MB | 806 MB |
| 读完全部历史 + 布局分支图 | 0.13 s | — |
| 打开 20 MB 补丁的提交（5677 个文件） | 0.24 s，+34 MB | — |
| 空闲 CPU | 0% | — |

## 开发与发布

```bash
go tool mygo dev      # 开发，改动自动重新加载
go test ./...         # 测试（界面测试不需要窗口）
go tool mygo build -platform darwin/arm64,windows/amd64,linux/amd64
```

文案在 `locales/`：`en.json` 是源语言，`zh-Hans.json` 是简体中文，改完运行 `go tool mygo generate` 重新生成 `locales/catalog_gen.go`。

发布：在 `CHANGELOG.md` 写好这个版本的说明，再推送版本标签，例如 `git tag v0.2.0 && git push origin v0.2.0`。CI 用仓库 Secret `MYGO_UPDATER_PRIVATE_KEY` 签名，打包 macOS dmg、Windows 安装程序与 zip、Linux deb 与更新包，并创建 GitHub Release。签名私钥在本机 `~/Library/Application Support/mygo/update-keys/mygit/`，丢了已安装的应用就再也收不到更新。

注意：仓库是私有的时候，已安装的应用下载不到 Release 里的更新包。测试自动更新前，要么把仓库设为公开，要么把更新包放到公开的地址（mygo 支持 `updates.url` 或 S3 / Cloudflare R2）。

基准测试：`MYGIT_BENCH_REPO=<仓库> MYGIT_BENCH_COMMIT=<提交> go test -run '^$' -bench .`
