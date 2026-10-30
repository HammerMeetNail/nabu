import { test, expect } from "@playwright/test";
import { register, headers } from "./review-fixtures.js";

const localDate = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

// A household whose baby pack named two babies, and a cat. The babies'
// activities are named for them ("Feed Ada"), so Stats finds them by the
// template they came from.
async function twoBabies(page) {
  await register(page);
  const h = await headers(page);
  expect((await page.request.post("/api/household", { headers: h, data: { name: "Synthetic twins" } })).ok()).toBeTruthy();
  const added = await page.request.post("/api/chores/packs", {
    headers: h,
    data: { packs: [{ key: "baby", names: ["Ada", "Ben"] }, { key: "cat", names: ["Mongo"] }] },
  });
  expect(added.ok()).toBeTruthy();
  const chores = (await (await page.request.get("/api/chores")).json()).chores;
  const byName = Object.fromEntries(chores.map((c) => [c.name, c]));
  await page.reload();
  await expect(page.locator(".home-grid")).toBeVisible();
  return { h, byName };
}

async function log(page, h, chore, minutesAgo, fields = {}) {
  const at = new Date(Date.now() - minutesAgo * 60000);
  const res = await page.request.post("/api/logs", {
    headers: h,
    data: { choreId: chore.id, completedAt: at.toISOString(), date: localDate(at), hour: at.getHours(), ...fields },
  });
  expect(res.ok()).toBeTruthy();
}

const babyCard = (page, name) =>
  page.locator('[data-stats-section="baby"] .card').filter({ has: page.locator("h3", { hasText: new RegExp(`^${name}$`) }) });

test.describe("Stats baby cards", () => {
  test("each named baby gets a card of their own", async ({ page }) => {
    const { h, byName } = await twoBabies(page);
    const feedAda = byName["Feed Ada"], feedBen = byName["Feed Ben"];
    // Two of Ada's feeds an hour apart make one cluster-feeding gap; Ben has
    // one feed, so none.
    await log(page, h, feedAda, 90, { volumeML: 120 });
    await log(page, h, feedAda, 30, { volumeML: 60 });
    await log(page, h, byName["Change Ada"], 45);
    await log(page, h, feedBen, 20, { volumeML: 90 });

    const reads = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (url.pathname === "/api/stats/dashboard") reads.push(...url.searchParams.getAll("read"));
      if (url.pathname === "/api/stats/feeding-gaps") reads.push(`feeding-gaps${url.search}`);
    });
    await page.locator('[data-nav="stats"]').click();
    await expect(page.locator('[data-stats-section="baby"] h3')).toHaveText(["Ada", "Ben"]);

    const ada = babyCard(page, "Ada"), ben = babyCard(page, "Ben");
    await expect(ada.locator(".baby-col-title")).toHaveText(["🍼 Feed Ada", "👶 Change Ada", /Cluster Feeding/]);
    await expect(ben.locator(".baby-col-title")).toHaveText(["🍼 Feed Ben", "👶 Change Ben", /Cluster Feeding/]);
    await expect(ada.locator('.feeding-gaps-chart [data-action="scatter-tap"]')).toHaveCount(1);
    await expect(ben.locator(".feeding-gaps-chart")).toHaveCount(0);
    await expect(ben.locator(".baby-care-column").filter({ hasText: "Cluster Feeding" })).toContainText("No data");
    // Each card asks for its own baby's feeds.
    expect(reads.some((read) => read.startsWith(`feeding-gaps?choreId=${feedAda.id}&`))).toBe(true);
    expect(reads.some((read) => read.startsWith(`feeding-gaps?choreId=${feedBen.id}&`))).toBe(true);
    // The babies' feeds are on their cards, not on cards of their own; the
    // cat's feeds are no baby's.
    await expect(page.locator(`[data-stats-section="chore:${feedAda.id}"]`)).toHaveCount(0);
    await expect(page.locator('[data-stats-section="baby"]')).not.toContainText("Mongo");

    // Each chart keeps its own period, and each card its own dates.
    await ben.locator(`[data-action="stats-baby-period"][data-chore-id="${feedBen.id}"][data-period="weekly"]`).click();
    await expect(ben.locator(`[data-chore-id="${feedBen.id}"][data-period="weekly"]`)).toHaveAttribute("aria-pressed", "true");
    await expect(ada.locator(`[data-chore-id="${feedAda.id}"][data-period="daily"]`)).toHaveAttribute("aria-pressed", "true");
    const adaStart = await ada.getByLabel("Start date", { exact: true }).inputValue();
    await ben.locator('[data-action="stats-feeding-gaps-quick"][data-days="1"]').click();
    await expect(ben.getByLabel("Start date", { exact: true })).toHaveValue(localDate(new Date()));
    await expect(ada.getByLabel("Start date", { exact: true })).toHaveValue(adaStart);
    expect(adaStart).not.toBe(localDate(new Date()));
    await ada.getByRole("button", { name: "How to read this chart" }).click();
    await expect(ada.locator(".feeding-gaps-explainer--visible")).toHaveCount(1);
    await expect(ben.locator(".feeding-gaps-explainer--visible")).toHaveCount(0);
  });

  test("a renamed baby activity stays on its baby's card", async ({ page }) => {
    const { h, byName } = await twoBabies(page);
    const renamed = await page.request.patch(`/api/chores/${byName["Feed Ada"].id}`, { headers: h, data: { name: "Bottles" } });
    expect(renamed.ok()).toBeTruthy();
    await page.reload();
    await expect(page.locator(".home-grid")).toBeVisible();
    await page.locator('[data-nav="stats"]').click();
    await expect(babyCard(page, "Ada").locator(".baby-col-title")).toHaveText(["🍼 Bottles", "👶 Change Ada", /Cluster Feeding/]);
    await expect(page.locator(`[data-stats-section="chore:${byName["Feed Ada"].id}"]`)).toHaveCount(0);
  });
});
