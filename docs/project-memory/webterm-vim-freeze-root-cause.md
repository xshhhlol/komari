---
name: webterm-vim-freeze-root-cause
description: "网页终端 vim '卡死' 的真正根因：vite 默认 build target 降级重写 @xterm/xterm 6.0 的 ||= 语法产生 strict-mode ReferenceError；komari-web build.target 必须保持 ≥ es2021"
metadata: 
  node_type: memory
  type: project
  originSessionId: a35a985b-10cd-41ef-bb43-f4192a538c60
---

komari 网页终端「vim 卡死/无交互」（2026-06 首次出现，2026-07-08 定位并根治）的真正根因，不是 vim 等待终端探测应答：

**根因**：@xterm/xterm 6.0 发布的 `lib/xterm.mjs` 是预压缩产物，`requestMode`（DECRQM 处理器）里用了 ES2021 的 `r||={}`。komari-web 的 vite build 默认 target 含 firefox78（不支持 `||=`），esbuild 降级重写 + 变量重命名后产出 `(void 0||(i={}))`——对未声明变量 `i` 赋值，ESM 严格模式下抛 `ReferenceError: i is not defined`。vim 在 TERM=xterm* 下启动必发 DECRQM（`\e[?12$p`），一命中就把 xterm.js 解析器炸死 → 终端整体假死（vim 画了一半、按键无反应）。dev 模式（不压缩）不触发，所以只有生产构建出问题。

**修复**（komari-web radix `25fbdc2`，随面板 2.20.1 发布）：`vite.config.ts` `build.target: "es2022"`。**永远不要把这个 target 降回去**；升级 @xterm/xterm 后要重查（grep dist 产物有无 `void 0||(` 之类对未声明变量赋值的模式）。

**曾经的错误结论**（已纠正，勿再采信）：
- 「vim 卡死是 xterm.js 不应答 XTGETTCAP」→ 错。实测 vim 8.0/8.1/9.0/9.1 在完全无 XTGETTCAP 应答时也不卡。2.20.0 加的 DCS 0+r 应答保留（无害且让 vim 不用等超时），但它不是修复。
- 「agent TERM=screen-256color 修复了卡死」→ 只是规避：screen TERM 下 vim 不发 xterm 探测（含 DECRQM），绕开了前端崩溃点。agent 1.6.1 的 TERM=xterm-256color 是正确终态（vim 只在 xterm* 下自动开 bracketed paste，粘贴才不会被 autoindent 弄成阶梯）。

**验证方法**（都做过、可复用）：Playwright 真浏览器加载真实 /terminal 页 + 监听 WS 帧；`page.on("pageerror")` 一抓一个准。vite preview 已配好与 dev 相同的 /api 代理（设 `VITE_API_TARGET` 即可对压缩产物做 E2E）。服务器侧可在任一被控机 /tmp 下用 2.20.x 面板二进制 + agent 起隔离测试环境（KOMARI_LISTEN=127.0.0.1:PORT，首次启动 stdout 打印 admin 密码）。

相关：[[forked-repos-use-xshhhlol]]、[[purcarte-theme-build-deploy]]
