# 性能台

在真实浏览器里量 Studio 的流式渲染成本。`harness.tsx` 挂的是完整的 `App`，
事件流由驱动脚本注入 —— 不需要内核，也不会把固件带进生产包（`perf.html`
不是主构建的入口）。

必须按**生产构建**量：React 的开发期检查（`jsxDEV`、`validateProperty`）比
被测代码本身还贵，dev 下的数字会把结论带偏。

```bash
npx vite build --config vite.perf.config.ts     # 产出 dist-perf/
npx http-server dist-perf -p 4399 -s            # 或任意静态服务器
pnpm exec playwright install chromium           # 首次:npx 会另装一份版本对不上的

node perf/bench.mjs      # 规模基准：0/40/150/400 轮下的帧时间与脚本/样式/布局
node perf/scale.mjs      # 极长会话：400/2000/8000/20000 轮的节点数、堆内存、帧时间
node perf/switch.mjs     # 切会话：点开一份已经很长的记录，到它出现在眼前要多久
node perf/rail.mjs       # 左栏会话树：2×8 / 10×50 / 20×200 / 40×500 的节点数与响应
node perf/profile.mjs    # CPU 采样，按自耗时列热点
node perf/panes.mjs      # 并行窗格的代价：两个会话同时流式
node perf/verify.mjs     # 行为验证：跟随、块的装卸、切页往返、大会话树
node perf/panels.mjs     # 两侧栏：收起补间、拖动改宽、上下限、窄屏退场
node perf/look.mjs       # 外观验证：字号、界面缩放、自定义字体、壁纸上传与调节
node perf/lang.mjs       # 双语验证：英文启动、界面译文到位、切回中文
node perf/locale.mjs     # 跟随系统语言：中文各写法都归中文，其余归英文
node perf/i18n.mjs       # 词表守卫：源码用到的中文 key 与英文词表是否对得上
node perf/codes.mjs      # 码的守卫：内核发的每个拒绝码，前端都要有话说
node perf/reason.mjs     # 内核拒绝的双语落地：同一个码，中英各说各的
```

`lang.mjs` 从仓库外跑时用 `PERF_SRC` 指向 `src/`（它要读固件源码来决定
哪些中文不必翻）。`PERF_URL` 可以指向别的地址；`bench.mjs` 和 `scale.mjs` 另收 `PERF_SCALES`、
`PERF_DELTAS`，`rail.mjs` 收 `PERF_TREE`（`区数x每区会话数`，逗号分隔），
`switch.mjs` 收 `PERF_TURNS` 和 `PERF_STATUS_MS`——后者把内核 `/status` 的
往返摆进来，用来分清「等的是这份记录」还是「等的是一次状态读」。
性能台自己也读 `?ws=&sess=` 来决定左栏那棵树有多大。

## 判据

这几条是这套 UI 的性能契约，回归时先看它们：

- **脚本时间与会话长度无关。** 20000 轮和空会话应当在同一量级。一旦随轮次
  线性上涨，说明又有派生值挂回了 `items` 而不是 `revision`，或者哪个组件
  的 memo 被一个每帧新建的 prop 打掉了。
- **DOM 节点数与会话长度无关。** `scale.mjs` 里 20000 轮应当只有几千个节点。
  涨起来就是有列表在无上限地渲染——转录的块虚拟化失效，或者又多了一个像
  「待审改动」那样一行一条目的面板（那个面板曾经一家就占 28k 节点）。
- **左栏节点数与会话总数无关。** `rail.mjs` 里 40 区 × 500 会话应当在几千个
  节点、折叠一下在 120ms 内。这三处是同一个毛病的三个地方：任何"一条数据一
  个 DOM 行"的列表都要有上限或虚拟化。
- **长任务为 0。** 出现长任务意味着某一帧做了整条记录的活。
- **切一次会话，等的只能是那份记录。** `switch.mjs` 在 `PERF_STATUS_MS=0` 与
  `=100` 两次之间的差值，就是有多少等待其实是在等一次状态读——那笔账会一比一
  落到「到内容」上。转录的恢复不该和状态读绑在同一个 `Promise.all` 里：记录已
  经到手就该画出来，状态晚一点到只是状态栏晚一点更新。
- **折条目的耗时不该随会话长度加速上涨。** `fromHistory` 里每来一条工具结果就
  线性找一次配对、每折一个条目就复制一次已折好的数组，两处都是 O(n²)；轮次翻
  倍而耗时涨到近四倍，就是它们又回来了。
- **采样里不该有 `get scrollHeight`。** 它是强制同步布局，跟随滚动一旦回到
  读几何属性的写法，就会每帧把整条记录重新排版一遍。读 `scrollTop` 不要紧，
  强制布局的是 `scrollHeight`/`clientHeight`/`offsetHeight`。
- **块的装卸必须收敛。** `verify.mjs` 会检查远处的块已卸载。历史教训：给每个
  块各建一个 IntersectionObserver 时，几百个实例的回调并不可靠，块会滞留在
  视口外 40 万像素处不卸载——必须是一个观察器观察多个目标。
- **栏收起时看得见地收。** `panels.mjs` 量的是那 45 帧里落在两头之间的宽度有
  几个，因为这件事退化时不报错。补间挂在 `--rail-w`/`--side-w` 上——两个用
  `@property` 注册成长度的变量；`grid-template-columns` 自己插不了值，那一列
  里有 `minmax(0, 1fr)`，而它只在纯长度之间可动画，写上 transition 也只会安静
  地退回跳变。拖动期间必须断开补间（`.app[data-drag]`），否则栏是追着指针走的。
- **没选过语言就跟机器走。** 判据在 `i18n/index.ts` 的 `isChinese`：以 zh /
  yue / cmn 开头，或含 hans / hant，都是中文；其余一律英文——英文是兜底，不是
  对机器语言的判断。只看首选语言：一台法语机器把中文排第二，答案仍是英文。
- **内核记着的语言压过本地缓存。** 本地那份只是启动时不闪的缓存；换机器、
  清缓存时靠 `adopt()` 采纳 config 里的选择，并且只重载一次就收敛。
- **内核不说人话。** 拒绝走 `refuse`/`busy`/`busyErr`/`refusal`，交出去的是
  码加参数；`error` 字段只是英文兜底，给日志和 curl 看，不是给用户看的译文。
  新增一条拒绝就要在 `i18n/kernel.ts` 里给它一句话 —— `perf/codes.mjs` 两侧
  都读源码来对，不是对一份手抄清单。
- **词表不能悄悄缺。** 缺翻译时界面退回中文而不是空白，所以漏了不会自己暴露，
  `perf/i18n.mjs` 是替代品：源码里每个 `t("…")` 的中文 key 都要在 `i18n/en.ts`
  里有。这条对应 internal/i18n 的 catalog 测试，理由一样。
- **跟随只由用户手势解除。** 用「底部标记离开视口」来解除是错的：块挂载会把
  底部推走，那样视口会被永久留在半空。
