// GFM ends a bare URL at ASCII punctuation, and nothing else. A Chinese
// sentence closes with full-width marks — "…127.0.0.1:8787（HTTP 200，正常）" —
// so the link swallowed the rest of the sentence up to the next space, and the
// underline ran on past the port. None of these ever appear inside a URL; CJK
// ideographs are deliberately not in the set, so an IDN host still autolinks.
const CJK_PUNCT = /[‘-‟…—　-〿！-･]/u;

interface Node {
  type: string;
  url?: string;
  value?: string;
  children?: Node[];
}

// remarkTrimAutolink cuts a literal autolink at the first character no URL can
// contain and hands the remainder back to the paragraph as text. An explicit
// [label](url) is left alone: there the author chose both halves.
export function remarkTrimAutolink() {
  return (tree: Node) => walk(tree);
}

function walk(node: Node) {
  const kids = node.children;
  if (!kids) return;
  for (let i = 0; i < kids.length; i++) {
    const n = kids[i];
    const cut = n.type === "link" && n.url ? literalCut(n) : -1;
    if (cut > 0 && n.url) {
      const rest = n.url.slice(cut);
      n.url = n.url.slice(0, cut);
      n.children = [{ type: "text", value: n.url }];
      kids.splice(i + 1, 0, { type: "text", value: rest });
      i++;
      continue;
    }
    walk(n);
  }
}

// A literal autolink renders its own URL as its only child; "www.x.com" also
// counts, where gfm prepends the scheme it left out.
function literalCut(link: Node): number {
  const text = link.children?.length === 1 && link.children[0].type === "text" ? link.children[0].value : undefined;
  if (!text || !link.url || (link.url !== text && !link.url.endsWith(text))) return -1;
  return link.url.search(CJK_PUNCT);
}
