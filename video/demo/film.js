// The film both cuts share: its words in each language, where things sit in
// the app's screenshots, and its timeline. Each cut (index.html, landscape;
// vertical.html, portrait) lays out the same elements and passes its own
// camera: what of the screenshots each beat frames.

const TEXT = {
  en: {
    tagline: "See what your agent changed",
    s2: "Changes show up as the agent works",
    s3: "Every change, side by side",
    s4: "Stage what's right",
    s5: "Hand the rest back to the agent",
    s6: "The IDE you know, without the IDE",
    s6sub: "JetBrains' graph and history, VS Code's staging",
    s7: "A fraction of the memory",
    note: "Memory with a repository of 20,000 commits open",
    ask: "add retries to fetchAll, with a test",
    reply: "After the third try this gives up without a word…",
  },
  zh: {
    tagline: "看清 agent 改了什么",
    s2: "agent 改着，\n改动就出现",
    s3: "看：每处改动，\n并排看清",
    s4: "留：没问题的，\n先暂存",
    s5: "退：其余的，\n交回给 agent",
    s6: "熟悉的 IDE 体验，\n不用打开 IDE",
    s6sub: "JetBrains 的分支图和历史，VS Code 的暂存方式",
    s7: "内存只要零头",
    note: "打开 2 万个提交的仓库后的内存占用",
    ask: "给 fetchAll 加上重试，再补个测试",
    reply: "试满三次就悄悄放弃了…",
  },
};

// Points in the 1568×1026 screenshots (the older ones are 1568×1045).
// The comments menu sits where its label ends, which differs by language.
const AT = {
  changesHead: [14, 689],
  rowApi: [40, 727],
  rowLock: [90, 761],
  rowTest: [60, 795],
  addLock: [343, 761],
  addTest: [343, 795],
  stagedHead: [14, 327],
  menu: { en: [1295, 28], zh: [1333, 28] },
  copy: { en: [1327, 66], zh: [1374, 66] },
};

function film(cam, register) {
  const vars = (window.__hyperframes && window.__hyperframes.getVariables()) || {};
  const lang = vars.lang === "zh" ? "zh" : "en";
  const root = document.getElementById("root");
  if (lang === "zh") root.classList.add("zh");
  document.querySelectorAll("[data-t]").forEach((el) => (el.textContent = TEXT[lang][el.dataset.t]));
  // The screenshots taken in the language (the compiler rewrites src, so
  // each names its shot).
  document.querySelectorAll("img[data-shot]").forEach((img) => (img.src = `assets/${lang}/${img.dataset.shot}.png`));

  // The camera: #shot holds the screenshots at their own pixels; a view
  // puts a point of them at the middle of the window, z times the scale
  // at which the window shows them whole across.
  const s0 = cam.width / 1568;
  function place(x, y, z) {
    const s = s0 * z;
    return { x: cam.width / 2 - x * s, y: cam.height / 2 - y * s, scale: s };
  }
  function view(tl, [x, y, z], at, dur = 0.9) {
    tl.to("#shot", { ...place(x, y, z), duration: dur, ease: "power3.inOut" }, at);
  }
  // cursor-click-ripple: the cursor's tip moves to a point, presses, rings out.
  function clickAt(tl, [x, y], at, moveDur = 0.6) {
    tl.to("#cursor", { x: x - 3, y: y - 2, duration: moveDur, ease: "power2.inOut" }, at - moveDur);
    tl.to("#cursor", { scale: 0.82, duration: 0.09, ease: "power2.in", yoyo: true, repeat: 1, transformOrigin: "3px 2px" }, at);
    tl.set("#ripple", { x, y, scale: 0.2, opacity: 1 }, at);
    tl.to("#ripple", { scale: 1.6, opacity: 0, duration: 0.5, ease: "power2.out" }, at);
  }
  function swap(tl, from, to, at, dur = 0.25) {
    tl.to(to, { opacity: 1, duration: dur, ease: "none" }, at);
    tl.set(from, { opacity: 0 }, at + dur);
  }
  function flash(tl, [x, y], w, at) {
    tl.set("#flash", { x: x - 10, y: y - 22, width: w, opacity: 0 }, at);
    tl.to("#flash", { opacity: 1, duration: 0.22, yoyo: true, repeat: 3 }, at);
  }
  function capIn(tl, sel, at) {
    tl.fromTo(`${sel} > *`, { opacity: 0, y: 24 }, { opacity: 1, y: 0, duration: 0.55, ease: "power3.out", stagger: 0.1 }, at);
  }
  function lines(tl, sel, at, gap = 0.45) {
    tl.fromTo(sel, { opacity: 0, x: -12 }, { opacity: 1, x: 0, duration: 0.3, ease: "power2.out", stagger: gap }, at);
  }

  function build() {
    const tl = gsap.timeline({ paused: true });

    // Background: the glows breathe and drift the whole film.
    tl.fromTo("#glow-a", { scale: 1, x: 0 }, { scale: 1.12, x: -80, duration: 21, ease: "sine.inOut", yoyo: true, repeat: 1 }, 0);
    tl.fromTo("#glow-b", { scale: 1.05, y: 0 }, { scale: 0.92, y: -60, duration: 21, ease: "sine.inOut", yoyo: true, repeat: 1 }, 0);

    // 1 · logo-assemble-lockup: the body blooms, the branches draw down
    // and meet, the merge pops, the words arrive.
    document.querySelectorAll(".m-line").forEach((p) => {
      const len = p.getTotalLength();
      tl.set(p, { strokeDasharray: len, strokeDashoffset: len }, 0);
    });
    tl.fromTo("#m-body", { opacity: 0, scale: 0.9, transformOrigin: "512px 512px" }, { opacity: 1, scale: 1, duration: 0.7, ease: "power3.out" }, 0.1);
    tl.fromTo(".m-dot", { scale: 0, transformOrigin: "center" }, { scale: 1, duration: 0.45, ease: "back.out(2)", stagger: 0.06 }, 0.35);
    tl.to(".m-line", { strokeDashoffset: 0, duration: 0.8, ease: "power2.inOut", stagger: 0.08 }, 0.55);
    tl.fromTo("#m-merge", { scale: 0, transformOrigin: "512px 584px" }, { scale: 1, duration: 0.5, ease: "back.out(2.6)" }, 1.35);
    tl.fromTo("#s1 .name", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.7, ease: "power3.out" }, 1.45);
    tl.fromTo("#s1 .tag", { opacity: 0, y: 16 }, { opacity: 1, y: 0, duration: 0.6, ease: "power2.out" }, 1.8);
    tl.to("#s1 > *", { opacity: 0, y: -24, duration: 0.4, ease: "power2.in" }, 3.2);

    // 2 · The agent works in its terminal; the window keeps up, file by file.
    tl.set("#shot", place(...cam.agent), 3.4);
    tl.fromTo("#app", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 3.5);
    capIn(tl, "#cap2", 3.6);
    tl.fromTo("#term", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.5, ease: "power3.out" }, 3.9);
    lines(tl, "#term .ask", 4.3);
    lines(tl, "#term .work", 5.0, 1.3);
    swap(tl, "#shot-clean", "#shot-one", 5.6);
    flash(tl, AT.changesHead, 140, 5.7);
    swap(tl, "#shot-one", "#shot-three", 8.0);
    flash(tl, AT.changesHead, 140, 8.1);
    tl.to("#cap2, #term", { opacity: 0, duration: 0.3 }, 10.6);

    // 3 · See: a file's diff, side by side; the lockfile folded away.
    capIn(tl, "#cap3", 11.0);
    tl.set("#cursor", { x: 700, y: 900 }, 10.9);
    tl.to("#cursor", { opacity: 1, duration: 0.2 }, 11.0);
    clickAt(tl, AT.rowApi, 11.7, 0.6);
    swap(tl, "#shot-three", "#shot-diff", 11.8);
    tl.to("#cursor", { opacity: 0, duration: 0.2 }, 12.0);
    view(tl, cam.diff, 12.2, 1.0);
    view(tl, cam.agent, 14.4, 0.8);
    tl.to("#cursor", { opacity: 1, duration: 0.2 }, 14.6);
    clickAt(tl, AT.rowLock, 15.4, 0.6);
    swap(tl, "#shot-diff", "#shot-folded", 15.5);
    view(tl, cam.folded, 15.7, 0.8);
    tl.to("#cap3", { opacity: 0, duration: 0.3 }, 16.7);

    // 4 · Keep: the lockfile and the test are right; stage them.
    capIn(tl, "#cap4", 17.0);
    view(tl, cam.agent, 16.9, 0.7);
    clickAt(tl, AT.addLock, 18.0, 0.7);
    clickAt(tl, AT.addTest, 18.7, 0.4);
    swap(tl, "#shot-folded", "#shot-staged", 18.8);
    view(tl, cam.staged, 19.1, 0.8);
    flash(tl, AT.stagedHead, 200, 19.6);
    tl.to("#cursor", { opacity: 0, duration: 0.2 }, 19.3);
    tl.to("#cap4", { opacity: 0, duration: 0.3 }, 21.7);

    // 5 · Send back: a comment on the lines to redo, copied as Markdown and
    // pasted to the agent.
    capIn(tl, "#cap5", 22.0);
    swap(tl, "#shot-staged", "#shot-comment", 22.2, 0.35);
    view(tl, cam.comment, 22.3, 1.0);
    view(tl, cam.menu, 24.6, 0.9);
    tl.to("#cursor", { opacity: 1, duration: 0.2 }, 24.9);
    clickAt(tl, AT.menu[lang], 25.6, 0.6);
    swap(tl, "#shot-comment", "#shot-menu", 25.7, 0.15);
    clickAt(tl, AT.copy[lang], 26.4, 0.4);
    swap(tl, "#shot-menu", "#shot-copied", 26.5, 0.15);
    tl.to("#cursor", { opacity: 0, duration: 0.2 }, 26.8);
    view(tl, cam.copied, 26.7, 0.8);
    tl.set("#term .ask, #term .work", { opacity: 0 }, 27.0);
    tl.fromTo("#term", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.5, ease: "power3.out" }, 27.3);
    lines(tl, "#term .paste", 27.6, 0.12);
    lines(tl, "#term .fix", 28.8);
    tl.to("#cap5, #term", { opacity: 0, duration: 0.3 }, 29.7);

    // 6 · The rest of a git client, as an IDE's: the graph, the branches,
    // the three-pane merge.
    capIn(tl, "#cap6", 30.0);
    swap(tl, "#shot-copied", "#shot-main", 29.9, 0.3);
    tl.set("#shot", place(...cam.main), 29.9);
    view(tl, cam.graph, 30.3, 1.0);
    swap(tl, "#shot-main", "#shot-feat", 31.5, 0.25);
    tl.set("#shot", place(...cam.feat), 31.5);
    swap(tl, "#shot-feat", "#shot-merged", 32.6, 0.25);
    tl.set("#shot", place(...cam.merged), 32.6);
    view(tl, cam.mergedNear, 32.7, 1.1);
    tl.to("#cap6", { opacity: 0, duration: 0.3 }, 33.7);
    tl.to("#app", { opacity: 0, scale: 0.96, duration: 0.4, ease: "power2.in" }, 33.7);

    // 7 · dataviz-countup with stat-bars-and-fills: the bars grow and their
    // numbers count up.
    tl.fromTo("#s7 > *", { opacity: 0, y: 30 }, { opacity: 1, y: 0, duration: 0.5, ease: "power3.out", stagger: 0.08 }, 34.1);
    const bars = [["#fill-me", "#v-me", 109], ["#fill-code", "#v-code", 1000], ["#fill-idea", "#v-idea", 2300]];
    const count = { me: 0, code: 0, idea: 0 };
    bars.forEach(([fill, , mb]) => tl.fromTo(fill, { scaleX: 0 }, { scaleX: mb / 2300, duration: 1.5, ease: "power2.out" }, 34.5));
    tl.fromTo(count, { me: 0, code: 0, idea: 0 }, {
      me: 109, code: 1000, idea: 2300, duration: 1.5, ease: "power2.out",
      onUpdate: () => {
        document.getElementById("v-me").textContent = Math.round(count.me).toLocaleString("en");
        document.getElementById("v-code").textContent = Math.round(count.code).toLocaleString("en");
        document.getElementById("v-idea").textContent = Math.round(count.idea).toLocaleString("en");
      },
    }, 34.5);
    tl.to("#s7", { opacity: 0, duration: 0.35 }, 37.65);

    // 8 · The end card: the logo settles, the words, the chips, the address.
    tl.fromTo("#end-mark", { opacity: 0, scale: 0.8 }, { opacity: 1, scale: 1, duration: 0.7, ease: "back.out(1.8)" }, 38.1);
    tl.fromTo("#s8 .name", { opacity: 0, y: 24 }, { opacity: 1, y: 0, duration: 0.6, ease: "power3.out" }, 38.4);
    tl.fromTo("#s8 .tag", { opacity: 0, y: 16 }, { opacity: 1, y: 0, duration: 0.5, ease: "power2.out" }, 38.7);
    tl.fromTo(".chip", { opacity: 0, y: 20 }, { opacity: 1, y: 0, duration: 0.45, ease: "power2.out", stagger: 0.09 }, 39.0);
    tl.fromTo(".url", { opacity: 0 }, { opacity: 1, duration: 0.6 }, 39.6);

    register(tl);
  }

  // Build once the screenshots of the language and the fonts are ready.
  Promise.all([...document.images].map((img) => (img.decode ? img.decode().catch(() => {}) : null)).concat(document.fonts.ready)).then(build);
}
