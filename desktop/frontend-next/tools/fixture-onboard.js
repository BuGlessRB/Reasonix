// Clicks the fixture through its connect card. The demo opens on that card and
// advances on what the probe found, so a sweep that wants the workbench behind
// it has to answer the form first. Fed to audit-sweep.mjs as ONB=this file.
(async () => {
  const set = (el, v) => {
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el), "value").set.call(el, v);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  };
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  for (const el of document.querySelectorAll("input")) {
    const h = el.name + el.id + (el.placeholder || "") + (el.type || "");
    if (/password|key/i.test(h)) set(el, "sk-audit-mock-key");
    else if (/url|address|http/i.test(h)) set(el, "https://api.deepseek.com");
  }
  await sleep(200);
  document.querySelector("button.onb-go")?.click();   // connect → probe
  await sleep(1800);
  const s1 = document.body.innerText.slice(0, 200);
  document.querySelector("button.onb-go")?.click();   // start
  await sleep(1500);
  return { afterProbe: s1.slice(0, 80), go: document.querySelector("button.onb-go")?.textContent };
})()
