// 左栏自己的记号，画在 Sym 那张 16 格网格上，描边同为 1.5。它们替掉的是两个
// 字符：`✎` 出自等宽栈、11px，`×` 出自界面栈、15px，而紧挨着的「+」是 13px 的
// 描边图形 —— 一排三个记号出自三种笔法，最重的那个恰好是删除。
export const Pencil = () => (
  <svg viewBox="0 0 16 16" aria-hidden="true">
    <path d="M10.9 3.4a1.3 1.3 0 0 1 1.8 1.8l-6.7 6.7-2.4.6.6-2.4Z" />
  </svg>
);

export const Cross = () => (
  <svg viewBox="0 0 16 16" aria-hidden="true">
    <path d="M5 5 11 11M11 5 5 11" />
  </svg>
);
