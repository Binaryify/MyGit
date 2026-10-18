// MyGit's site: its two languages, the rail that draws the page's commit
// graph as it scrolls, and the reveals.

const zh = {
  "nav.why": "为什么",
  "nav.features": "功能",
  "nav.install": "安装",
  "hero.title": "<span class=\"line\">只要提交图，</span><br><em class=\"line\">不要整个 IDE。</em>",
  "hero.lede": "MyGit 是轻量、快速的原生界面 git 客户端：JetBrains 的历史操作，VS Code 的暂存方式，内存只要六分之一。",
  "hero.get": "获取 MyGit",
  "hero.platforms": "macOS · Windows · Linux",
  "why.title": "内存只要六分之一。",
  "why.lede": "基于 IDE 平台的 git 客户端，会把整个 IDE 一起带上。MyGit 是一个原生二进制文件，由 MyGo 用 GPU 绘制界面：没有 webview，也没有 JVM。",
  "why.other": "IntelliJ 平台的客户端",
  "why.note": "打开一个 2 万个提交的仓库后的内存占用。",
  "why.s1": "读完 2 万个提交并布局分支图",
  "why.s2": "打开一个 20 MB 补丁的提交",
  "why.s3": "空闲时的 CPU",
  "why.s4": "完成每一个改动，表现和终端里完全一致",
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
  "every.title": "一个应用，在哪儿都能用。",
  "every.mac": "Apple 芯片",
  "every.lang": "跟随系统语言",
  "every.theme": "深色与浅色",
  "every.title2": "自定义标题栏",
  "every.update": "签名的自动更新",
  "every.update2": "一键完成",
  "install.title": "获取 MyGit。",
  "install.lede": "安装包即将发布。在那之前，可以用 Go 1.27 或更新版本从源码构建，需要 PATH 里有 git。",
  "install.copy": "复制",
  "install.copied": "已复制",
  "keys.title": "快捷键",
  "keys.commit": "提交",
  "keys.search": "搜索历史",
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
    if (s === undefined) return;
    if (s.includes("<")) el.innerHTML = s;
    else el.textContent = s;
  });
  document.querySelectorAll("[data-i18n-html]").forEach((el) => {
    const s = t[el.dataset.i18nHtml];
    if (s !== undefined) el.innerHTML = s;
  });
  // The screenshots show the app in the same language.
  document.querySelectorAll("img[data-shot]").forEach((img) => {
    img.src = `assets/${img.dataset.shot}-${next}.webp`;
  });
  document.querySelectorAll(".lang button").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.lang === next)));
  try { localStorage.setItem("mygit-lang", next); } catch {}
  requestAnimationFrame(layoutRail);
}
document.querySelectorAll(".lang button").forEach((b) => b.addEventListener("click", () => setLang(b.dataset.lang)));
setLang(preferredLang());

// The rail: a blue branch from the hero's commit through every section to
// the footer's merge, and a green one that forks off beside the features
// and merges back at the end — drawn as far as the page has scrolled.
const rail = document.querySelector(".rail");
const blue = rail.querySelectorAll(".rail-track:not(.rail-track-green), .rail-draw:not(.rail-draw-green)");
const green = rail.querySelectorAll(".rail-track-green, .rail-draw-green");
const drawBlue = rail.querySelector(".rail-draw:not(.rail-draw-green)");
const drawGreen = rail.querySelector(".rail-draw-green");

function railX() {
  return parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--rail-x")) || 56;
}

function layoutRail() {
  const x = railX();
  const top = (el) => el.getBoundingClientRect().top + window.scrollY;
  const hero = document.querySelector(".hero");
  const features = document.querySelector("#features");
  const everywhere = document.querySelector(".everywhere");
  const merge = document.querySelector(".merge-dot");
  const height = document.documentElement.scrollHeight;
  rail.setAttribute("height", height);
  rail.setAttribute("viewBox", `0 0 ${window.innerWidth} ${height}`);

  const y0 = top(hero) + 27;
  const yEnd = top(merge) + 16;
  const yFork = top(features) + 15;
  const yJoin = top(everywhere) + 15;
  const gx = x + 26; // the green branch's lane
  const b = `M ${x} ${y0} V ${yEnd}`;
  const g = `M ${x} ${yFork} C ${x} ${yFork + 60}, ${gx} ${yFork + 40}, ${gx} ${yFork + 110} V ${yJoin - 110} C ${gx} ${yJoin - 40}, ${x} ${yJoin - 60}, ${x} ${yJoin}`;
  blue.forEach((p) => p.setAttribute("d", b));
  green.forEach((p) => p.setAttribute("d", g));
  for (const p of [drawBlue, drawGreen]) {
    const len = p.getTotalLength();
    p.style.strokeDasharray = `${len}`;
    p.dataset.len = len;
  }
  drawRail();
}

function drawRail() {
  // The line reaches a little past the middle of the window.
  const reach = window.scrollY + window.innerHeight * 0.62;
  for (const p of [drawBlue, drawGreen]) {
    const len = +p.dataset.len || 0;
    const box = p.getBBox();
    const done = Math.max(0, Math.min(1, (reach - box.y) / Math.max(box.height, 1)));
    p.style.strokeDashoffset = `${len * (1 - done)}`;
  }
}

window.addEventListener("scroll", drawRail, { passive: true });
window.addEventListener("resize", () => requestAnimationFrame(layoutRail));
window.addEventListener("load", layoutRail);
document.fonts && document.fonts.ready.then(layoutRail);
layoutRail();

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
  { threshold: 0.2 }
);
document.querySelectorAll(".reveal").forEach((el) => io.observe(el));

function countUp(el) {
  const to = +el.dataset.to;
  const start = performance.now();
  const step = (now) => {
    const t = Math.min(1, (now - start) / 1600);
    el.textContent = Math.round(to * (1 - Math.pow(1 - t, 3)));
    if (t < 1) requestAnimationFrame(step);
  };
  if (matchMedia("(prefers-reduced-motion: reduce)").matches) el.textContent = to;
  else requestAnimationFrame(step);
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
