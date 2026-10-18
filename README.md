# mygit

轻量的 git 客户端，用 [MyGo](https://github.com/egoist/mygo) 原生 UI 写成，交互参考 IntelliJ / Rebased 的 git 工具。

- 历史：所有分支、标签、远程，带分支图，流式加载
- 提交详情：改动的文件 + 并排 diff（语法高亮）
- 工作区：暂存 / 取消暂存（行内按钮、双击、右键菜单），提交、修改上次提交
- 分支切换，获取 / 拉取（仅快进）/ 推送
- 读取靠解析 `git` 的输出，写操作全部交给 `git` 命令本身

```bash
go run . ~/path/to/repo
```

## 性能（Apple M4，2 万个提交的仓库）

| | mygit | Rebased |
|---|---|---|
| 打开后内存（footprint） | 109 MB | 806 MB |
| 读完全部历史 + 布局分支图 | 0.13 s | — |
| 打开 20 MB 补丁的提交（5677 个文件） | 0.24 s，+34 MB | — |
| 空闲 CPU | 0% | — |

基准测试：`MYGIT_BENCH_REPO=<仓库> MYGIT_BENCH_COMMIT=<提交> go test -run '^$' -bench .`
