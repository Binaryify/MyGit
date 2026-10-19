// MyGit's site: its two languages, the hero shot settling as the page
// scrolls, the cards' pointer light, the demo's play button, and reveals.

const zh = {
  "nav.demo": "演示",
  "nav.why": "为什么",
  "nav.features": "功能",
  "nav.install": "安装",
  "hero.pill": "原生 · GPU 绘制 · 极低的内存占用",
  "hero.title": "<span class=\"nw\">git 历史</span><span class=\"nw\">一目了然，</span><br><span class=\"grad nw\">不必打开 IDE。</span>",
  "hero.lede": "现代、轻量、启动即用的原生界面 git 客户端：JetBrains 的历史操作，VS Code 的暂存方式，内存只要六分之一。",
  "hero.get": "获取 MyGit",
  "hero.watch": "观看演示",
  "hero.platforms": "macOS · Windows · Linux",
  "demo.eyebrow": "演示",
  "demo.title": "40 秒看懂 MyGit。",
  "why.eyebrow": "性能",
  "why.title": "内存只要六分之一。",
  "why.lede": "基于 IDE 平台的 git 客户端，会把整个 IDE 一起带上。MyGit 是一个原生二进制文件，由 MyGo 用 GPU 绘制界面：没有 webview，也没有 JVM，所以点开就能用，不用等平台启动，也不用预热。",
  "why.other": "精简版 IntelliJ 客户端",
  "why.code": "VS Code，不带插件",
  "why.codeExt": "VS Code，167 个插件",
  "why.note": "打开一个 2 万个提交的仓库后的内存占用，应用的所有进程都计算在内。",
  "why.s1": "读完 2 万个提交并布局分支图",
  "why.s2": "打开一个 20 MB 补丁的提交",
  "why.s3": "空闲时的 CPU",
  "why.s4": "完成每一个改动，表现和终端里完全一致",
  "vs.eyebrow": "对比",
  "vs.title": "为什么不用 Sourcetree？",
  "vs.lede": "Sourcetree 免费、成熟，但和 MyGit 想做的不是一回事。",
  "vs.linux.a": "支持",
  "vs.linux.b": "不支持，只有 macOS 和 Windows",
  "vs.same": "各平台同一个应用",
  "vs.same.a": "同一套代码、同样的功能",
  "vs.same.b": "macOS 和 Windows 是两个不同的应用，功能有差异",
  "vs.merge": "解决冲突",
  "vs.merge.a": "内置三栏，一个冲突一个冲突地处理",
  "vs.merge.b": "整个文件采用某一边，或交给外部合并工具",
  "vs.edit": "暂存前直接在 diff 里改一行",
  "vs.edit.a": "可以",
  "vs.edit.b": "不行",
  "features.eyebrow": "功能",
  "features.title": "常用的都在，用不上的一个没有。",
  "f1.title": "想暂存什么就暂存什么。",
  "f1.body": "按文件、改动块，甚至按行暂存。改动的文件整篇显示，发现错字可以直接在 diff 里改完保存，再只暂存你想要的那几行。",
  "f2.title": "三栏对照，解决冲突。",
  "f2.body": "我们的、结果、他们的，三栏并排。一个冲突一个冲突地采用某一边或两边都要，还能直接编辑结果，然后在顶部的状态条里继续合并、变基或 cherry-pick。",
  "f3.title": "任何分支都能找到。",
  "f3.body": "边输入边搜索本地和远程分支，在每个分支的菜单里切换、合并、变基、比较、重命名或删除，也能从任意提交开始交互式变基。",
  "f4.title": "怎么打开都行。",
  "f4.body": "把文件夹拖到窗口上、克隆、新建仓库，或者在终端里运行 <code>mygit .</code>。每个仓库都有自己的窗口。",
  "m1": "按说明、作者、哈希搜索历史，按分支或路径筛选",
  "m2": "带语法高亮的并排 diff，⌘ 点选两个提交即可比较",
  "m3": "拖动重排，改写说明、squash、fixup 或丢弃",
  "m4": "stash 可包括未跟踪文件，可应用、弹出或从 stash 建分支",
  "m5": "每一行是谁改的，双击直达对应提交",
  "m6": "推送前预览要发送的提交，force-with-lease，需要登录时在窗口里询问",
  "m7": "生成的文件默认折叠，可标记已看，图片前后对比，评论可复制为 Markdown",
  "m8": "审阅 GitHub 拉取请求：讨论显示在对应行下，可回复、批准",
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
  "install.lede": "安装包即将发布。在那之前，可以用 Go 1.27 或更新版本从源码构建，需要 PATH 里有 git。",
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

// The nav gains its rule once the page moves, and the hero shot starts
// tilted back and lies flat as it comes up the window.
const nav = document.querySelector(".nav");
const shot = document.querySelector(".hero-shot");
const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
function onScroll() {
  nav.classList.toggle("scrolled", window.scrollY > 8);
  if (still) return;
  const top = shot.getBoundingClientRect().top;
  const t = Math.max(0, Math.min(1, (top - window.innerHeight * 0.15) / (window.innerHeight * 0.6)));
  shot.style.setProperty("--tilt", `${(t * 14).toFixed(2)}deg`);
}
window.addEventListener("scroll", onScroll, { passive: true });
window.addEventListener("resize", onScroll);
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

// Copying the build commands, without the prompts and the comment.
document.querySelector(".copy").addEventListener("click", async (e) => {
  const text = "git clone https://github.com/Binaryify/MyGit.git\ncd MyGit\ngo tool mygo build\n";
  try {
    await navigator.clipboard.writeText(text);
    e.target.textContent = (lang === "zh" ? zh : en)["install.copied"];
    setTimeout(() => (e.target.textContent = (lang === "zh" ? zh : en)["install.copy"]), 1600);
  } catch {}
});
