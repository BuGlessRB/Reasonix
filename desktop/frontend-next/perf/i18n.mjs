// 词表守卫：源码里每个 t("…") 的中文 key，英文词表都要有；反过来词表里也不
// 该留下源码已经不用的条目。缺翻译时界面会退回中文而不是空白，正因如此漏了
// 不会自己暴露 —— 这个检查就是替代品，对应 internal/i18n 的 catalog 测试。
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const SRC = join(dirname(fileURLToPath(import.meta.url)), "..", "src");

function walk(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else if (/\.tsx?$/.test(name)) out.push(p);
  }
  return out;
}

// 词表本身是答案，扫它等于自问自答。词表不止 en.ts：en_kernel / en_remote /
// en_settings / en_window 都是它的一部分，而这里过去只排除了 en.ts —— 于是那三
// 张表里的每一条中文都被当成「源码在用、词表没有」，「缺译」那份名单里绝大多数
// 一直是这么来的。kernel.ts 不同，它的中文是码的原文，靠 t(变量) 消费。
const isTable = (f) => /[\\/]i18n[\\/]en(_[a-z]+)?\.ts$/.test(f);
const files = walk(SRC).filter((f) => !isTable(f) && !f.endsWith("i18n/index.ts"));
const CJK = /[一-鿿]/;

// 两类引用，判据不同：
//  强 —— t("中文")，key 就写在调用点上，必须有翻译，缺了就是错。
//  弱 —— 别处的中文字面量。常量表（档位名、分区名、状态名）走的是
//        t(变量)，静态分析看不见 key，所以这些串也要算「在用」，
//        否则它们的翻译会被误报成多余的。
const CALL = /\bt\(\s*(["'])((?:[^"'\\]|\\.)*?)\1/g;
const LITERAL = /(["'])((?:[^"'\\]|\\.)*?)\1/g;
const used = new Map();
const loose = new Set();
for (const f of files) {
  const src = readFileSync(f, "utf8");
  for (const m of src.matchAll(CALL)) {
    if (CJK.test(m[2])) used.set(m[2], (used.get(m[2]) ?? new Set()).add(f.slice(SRC.length + 1)));
  }
  for (const m of src.matchAll(LITERAL)) if (CJK.test(m[2])) loose.add(m[2]);
}

const KEY = /^\s{2}"((?:[^"\\]|\\.)*)":/gm;
const have = new Set(
  readdirSync(join(SRC, "i18n"))
    .filter((f) => /^en(_[a-z]+)?\.ts$/.test(f))
    .flatMap((f) => [...readFileSync(join(SRC, "i18n", f), "utf8").matchAll(KEY)].map((m) => m[1])),
);

const missing = [...used.keys()].filter((k) => !have.has(k)).sort();
const unused = [...have].filter((k) => CJK.test(k) && !used.has(k) && !loose.has(k)).sort();

// fixture 与开发用文案不进界面，不必翻。
console.log(`t() 直接用的中文 key: ${used.size}   常量表等间接引用: ${loose.size}   英文词表: ${have.size}`);
// 同上：扫空了不是「全都翻好了」。
if (!used.size || !have.size) {
  console.log("\n没扫到源码或词表：先确认 PERF_SRC 指对了地方。");
  process.exit(1);
}
if (missing.length) {
  console.log(`\n缺英文翻译 ${missing.length} 条：`);
  for (const k of missing.slice(0, 40)) console.log(`  ${k}    ← ${[...used.get(k)][0]}`);
  if (missing.length > 40) console.log(`  …还有 ${missing.length - 40} 条`);
}
if (unused.length) {
  console.log(`\n词表里源码已不用的 ${unused.length} 条（既没 t() 引用，也不在任何中文字面量里）：`);
  for (const k of unused.slice(0, 20)) console.log(`  ${k}`);
}
if (!missing.length && !unused.length) console.log("\n词表与源码一致");
process.exit(missing.length ? 1 : 0);
