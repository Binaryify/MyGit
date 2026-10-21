// MyGit's site: its two languages, the nav's rule as the page scrolls,
// the cards' pointer light, the demo's play button, and reveals.

const zh = {
  "nav.demo": "演示",
  "nav.why": "为什么",
  "nav.features": "功能",
  "nav.install": "安装",
  "hero.pill": "原生 · 安装包不到 7 MB · 本地和 SSH 远程",
  "hero.title": "<span class=\"nw\">轻快的原生 Git 客户端</span><br><span class=\"grad nw\">远程仓库也能用</span>",
  "hero.lede": "JetBrains 那样的分支图和历史，VS Code 那样的暂存，安装包不到 7 MB，秒开；本地和 SSH 远程的仓库都能用。也为 agent 写代码而做：终端里 <code>mygit .</code> 打开，agent 改到哪你看到哪。",
  "hero.get": "获取 MyGit",
  "hero.watch": "观看演示",
  "hero.platforms": "macOS · Windows · Linux",
  "demo.eyebrow": "演示",
  "demo.title": "一分钟内看懂 MyGit。",
  "flow.eyebrow": "为 agent 写代码而做",
  "flow.title": "用 git 的大部分时间，都在看改了什么。",
  "flow.lede": "MyGit 让这件事变快。它读的是仓库，不是某个 agent 的会话，所以换哪个 agent、同时用几个、还是你自己改的代码，用起来都一样。",
  "flow.s1.title": "看",
  "flow.s1.body": "文件一改，这里马上就能看到，不用手动刷新。每个文件并排对比、带语法高亮。锁文件和生成的文件自动折叠，图片改动显示前后对比。",
  "flow.s2.title": "留",
  "flow.s2.body": "直接在 diff 里修一行，把没问题的文件、改动块或行暂存起来，然后提交。",
  "flow.s3.title": "退",
  "flow.s3.body": "在要返工的行上写一句，复制为 Markdown（每条带文件、行号和代码），粘给 Claude Code、Codex 或同事。",
  "why.eyebrow": "轻量、快速、现代",
  "why.title": "熟悉的 IDE 体验，内存只要零头。",
  "why.lede": "JetBrains 的分支图和历史操作，VS Code 的改动列表和暂存方式，⌘K 打开命令栏，⌘P 转到文件，深浅色都有。但它不是 IDE：MyGit 是一个原生二进制文件，由 MyGo 用 GPU 绘制界面，没有 webview，也没有 JVM，安装包不到 7 MB，点开就能用。",
  "why.other": "精简版 IntelliJ 客户端",
  "why.code": "VS Code，不带插件",
  "why.codeExt": "VS Code，167 个插件",
  "why.note": "打开一个 2 万个提交的仓库后的内存占用，应用的所有进程都计算在内。",
  "why.s1": "读完 2 万个提交并布局分支图",
  "why.s2": "打开一个 20 MB 补丁的提交",
  "why.s3": "安装包大小",
  "why.s4": "完成每一个改动，表现和终端里完全一致",
  "vs.eyebrow": "对比",
  "vs.title": "现在你怎么看 agent 的改动？",
  "vs.today": "现在的办法",
  "vs.agent": "agent 自带的 diff 视图",
  "vs.agent.a": "哪个 agent 都能用，还能逐行暂存，提交前直接改代码",
  "vs.ide": "打开 IDE",
  "vs.ide.a": "点开就能用，内存只要零头",
  "vs.term": "终端里跑 <code>git diff</code>",
  "vs.term.a": "并排对比，生成文件自动折叠，文件一改马上能看到",
  "features.eyebrow": "功能",
  "features.title": "常用的都在，用不上的一个没有。",
  "f1.title": "想暂存什么就暂存什么。",
  "f1.body": "按文件、改动块，甚至按行暂存。改动的文件整篇显示，发现错字可以直接在 diff 里改完保存，再只暂存你想要的那几行。",
  "f2.title": "三栏对照，解决冲突。",
  "f2.body": "我们的、结果、他们的，三栏并排。一个冲突一个冲突地采用某一边或两边都要，还能直接编辑结果，然后在顶部的状态条里继续合并、变基或 cherry-pick。",
  "f3.title": "任何分支都能找到。",
  "f3.body": "边输入边搜索本地和远程分支，在每个分支的菜单里切换、合并、变基、比较、重命名或删除，也能从任意提交开始交互式变基。",
  "f4.title": "怎么打开都行。",
  "f4.body": "把文件夹拖到窗口上、克隆、新建仓库、在终端里运行 <code>mygit .</code>，或者通过 SSH 打开其他电脑上的仓库。每个仓库都有自己的窗口。",
  "m1": "按说明、作者、哈希搜索历史，按分支或路径筛选",
  "m2": "带语法高亮的并排 diff，⌘ 点选两个提交即可比较",
  "m3": "拖动重排，改写说明、squash、fixup 或丢弃",
  "m4": "stash 可包括未跟踪文件，可应用、弹出或从 stash 建分支",
  "m5": "每一行是谁改的，双击直达对应提交",
  "m6": "推送前预览要发送的提交，force-with-lease，需要登录时在窗口里询问",
  "m7": "生成的文件默认折叠，可标记已看，图片前后对比，评论可复制为 Markdown",
  "m8": "审阅 GitHub 拉取请求：讨论显示在对应行下，可回复、批准",
  "m10": "一个窗口打开整个文件夹里的仓库：每个仓库的改动、提交框和同步都列在侧边栏，像 VS Code 的源代码管理",
  "m9": "通过 SSH 打开其他电脑上的仓库：Linux、macOS、Windows 都行，对方只要有 git",
  "every.eyebrow": "跨平台",
  "every.title": "一个应用，在哪儿都能用。",
  "every.mac": "Apple 芯片",
  "every.lang": "跟随系统语言",
  "every.theme": "深色与浅色",
  "every.title2": "自定义标题栏",
  "every.update": "签名的自动更新",
  "every.update2": "一键完成",
  "install.eyebrow": "安装",
  "install.title": "获取 MyGit。",
  "install.lede": "免费开源。需要 PATH 里有 git，安装后会自动更新。",
  "install.mac": "Apple 芯片",
  "install.dmg": "下载 .dmg",
  "install.exe": "安装程序",
  "install.linux": "x64 · 或用下面的脚本",
  "install.notarize": "暂未经过 Apple 公证。如果 macOS 无法打开，运行",
  "install.smartscreen": "Windows 安装程序暂时也没有代码签名。如果 Windows 提示「已保护你的电脑」，点「更多信息」，再点「仍要运行」。",
  "install.script": "# 装到 ~/.local，不需要 root",
  "install.all": "所有版本 →",
  "install.copy": "复制",
  "install.copied": "已复制",
  "keys.title": "快捷键",
  "keys.commit": "提交",
  "keys.search": "搜索历史",
  "keys.file": "转到文件",
  "keys.commands": "命令栏",
  "keys.stage": "暂存或取消暂存选中的改动",
  "keys.discard": "丢弃选中的改动",
  "keys.save": "保存在 diff 里编辑的文件",
  "keys.compare": "比较两个提交",
  "keys.note": "Windows 和 Linux 上用 Ctrl 代替 ⌘。",
  "footer.made": "用 Go 和 <a href=\"https://github.com/egoist/mygo\">MyGo</a> 打造。",
};

// The English texts are the page's own, kept as they load.
const en = {};
document.querySelectorAll("[data-i18n]").forEach((el) => (en[el.dataset.i18n] = el.textContent));
document.querySelectorAll("[data-i18n-html]").forEach((el) => (en[el.dataset.i18nHtml] = el.innerHTML));
en["install.copied"] = "Copied";

function preferredLang() {
  try {
    const saved = localStorage.getItem("mygit-lang");
    if (saved === "en" || saved === "zh") return saved;
  } catch {}
  return (navigator.language || "").toLowerCase().startsWith("zh") ? "zh" : "en";
}

let lang = "en";
function setLang(next) {
  lang = next;
  const t = next === "zh" ? zh : en;
  document.documentElement.lang = next === "zh" ? "zh-CN" : "en";
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    const s = t[el.dataset.i18n];
    if (s !== undefined) el.textContent = s;
  });
  document.querySelectorAll("[data-i18n-html]").forEach((el) => {
    const s = t[el.dataset.i18nHtml];
    if (s !== undefined) el.innerHTML = s;
  });
  // The screenshots and the demo show the app in the same language.
  document.querySelectorAll("img[data-shot]").forEach((img) => {
    img.src = `assets/${img.dataset.shot}-${next}.webp`;
  });
  document.querySelectorAll("video[data-video]").forEach((v) => {
    v.poster = `assets/${v.dataset.video}-${next}.webp`;
    v.src = `assets/${v.dataset.video}-${next}.mp4`;
    v.controls = false;
    v.closest(".video-frame").classList.remove("playing");
  });
  document.querySelectorAll(".lang button").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.lang === next)));
  try { localStorage.setItem("mygit-lang", next); } catch {}
}
document.querySelectorAll(".lang button").forEach((b) => b.addEventListener("click", () => setLang(b.dataset.lang)));
setLang(preferredLang());

// The nav gains its rule once the page moves.
const nav = document.querySelector(".nav");
function onScroll() {
  nav.classList.toggle("scrolled", window.scrollY > 8);
}
window.addEventListener("scroll", onScroll, { passive: true });
onScroll();

// The light under the pointer on each card.
document.querySelectorAll(".card").forEach((card) => {
  card.addEventListener("pointermove", (e) => {
    const r = card.getBoundingClientRect();
    card.style.setProperty("--mx", `${e.clientX - r.left}px`);
    card.style.setProperty("--my", `${e.clientY - r.top}px`);
  });
});

// The demo plays from its own button, then hands over to the controls.
document.querySelectorAll(".video-frame").forEach((frame) => {
  const video = frame.querySelector("video");
  frame.querySelector(".play").addEventListener("click", () => {
    frame.classList.add("playing");
    video.controls = true;
    video.play();
  });
});

// Reveals, and the memory bars counting up as they arrive.
const io = new IntersectionObserver(
  (entries) => {
    for (const e of entries) {
      if (!e.isIntersecting) continue;
      e.target.classList.add("in");
      e.target.querySelectorAll(".count").forEach(countUp);
      io.unobserve(e.target);
    }
  },
  { threshold: 0.15 }
);
document.querySelectorAll(".reveal").forEach((el) => io.observe(el));

const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
function countUp(el) {
  const to = +el.dataset.to;
  if (still) { el.textContent = to; return; }
  const start = performance.now();
  const step = (now) => {
    const t = Math.min(1, (now - start) / 1600);
    el.textContent = Math.round(to * (1 - Math.pow(1 - t, 3)));
    if (t < 1) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}

// Copying the install command, without the prompt and the comment.
document.querySelector(".copy").addEventListener("click", async (e) => {
  const text = "curl -fsSL https://github.com/Binaryify/MyGit/releases/latest/download/install.sh | sh\n";
  try {
    await navigator.clipboard.writeText(text);
    e.target.textContent = (lang === "zh" ? zh : en)["install.copied"];
    setTimeout(() => (e.target.textContent = (lang === "zh" ? zh : en)["install.copy"]), 1600);
  } catch {}
});
