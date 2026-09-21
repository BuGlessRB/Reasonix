// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import "../testkit";
import { NoticeCard } from "./NoticeCard";
import { t } from "../../i18n";
import type { Item } from "../../state/session";

afterEach(cleanup);

type Notice = Extract<Item, { t: "notice" }>;

const notice = (over: Partial<Notice> = {}): Notice => ({
  t: "notice",
  id: "n1",
  level: "warn",
  code: "verification_stalled",
  text: "The same check has failed 3 rounds running and still reports the same thing.",
  detail: "verification stalled: 3 rounds, 5 change(s) landed against it without moving it",
  ...over,
});

const draw = (over: Partial<Notice> = {}) => render(<NoticeCard item={notice(over)} />).container;

// The card is one of the host's, so it is drawn like the others: who is
// speaking, then the body. Both the headline and the gutter are new, and both
// are the difference between a warning and a paragraph a healthy runtime wrote.
describe("a notice card", () => {
  it("names its speaker rather than opening as a bare paragraph", () => {
    const box = draw();
    expect(box.querySelector(".hl .nm")?.textContent).toBe(t("警告"));
    expect(box.querySelector(".hl .tag")?.textContent).toBe(t("主机"));
  });

  it("wears its severity where the gutter can follow it", () => {
    expect(draw().querySelector(".call")?.getAttribute("data-lvl")).toBe("warn");
    expect(draw({ level: "error" }).querySelector(".call")?.getAttribute("data-lvl")).toBe("err");
    expect(draw({ level: "info" }).querySelector(".call")?.getAttribute("data-lvl")).toBeNull();
  });

  // The kernel writes its own English for the log. A code this build has a
  // sentence for is the one the reader gets; only an unknown code falls back.
  it("shows this build's sentence for a code it knows", () => {
    const said = draw().querySelector(".find .t")?.textContent ?? "";
    expect(said).toContain(t("同一个检查连着几轮都报同样的结果，是否继续由你定"));
    expect(said).not.toContain("The same check has failed");
  });

  it("keeps the kernel's diagnostic as the second half", () => {
    const why = draw().querySelector(".find .why")?.textContent ?? "";
    expect(why).toContain("3 rounds");
  });

  it("shows an unknown code's own English rather than nothing", () => {
    const box = draw({ code: "some_future_code", text: "something the kernel said" });
    expect(box.querySelector(".find .t")?.textContent).toContain("something the kernel said");
  });
});
