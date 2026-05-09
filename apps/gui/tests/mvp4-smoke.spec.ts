import { expect, test } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const projectRoot = process.env.ARTEMIS_E2E_PROJECT_ROOT ?? createTempProject("artemis-mvp4-");

function createTempProject(prefix: string) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  fs.writeFileSync(path.join(root, "README.md"), "Artemis MVP 4 GUI smoke project\n", "utf8");
  fs.mkdirSync(path.join(root, "docs"));
  fs.writeFileSync(
    path.join(root, "docs", "artemis_mvp4.md"),
    "Brainstorming Room and Decision Record\n",
    "utf8"
  );
  return root;
}

test("runs the MVP 4 brainstorming decision and conversion flow", async ({ page }) => {
  const projectName = `Artemis MVP4 ${Date.now()}`;

  await page.goto("/");
  await expect(page.getByText("online")).toBeVisible();

  await page.getByLabel("Name").fill(projectName);
  await page.getByLabel("Root Path").fill(projectRoot);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(page.getByRole("button", { name: new RegExp(projectName) })).toBeVisible();

  await page.getByLabel("Title").fill("MVP 4 E2E Smoke");
  await page.getByRole("button", { name: "Create" }).click();

  await page
    .getByPlaceholder("Frame the decision or design topic.")
    .fill("Review MVP 4 Brainstorming Room and Decision Record scope.");
  await page.getByRole("button", { name: "Start Brainstorming" }).click();

  await expect(page.getByText("brainstorming.decision_brief_created")).toBeVisible({
    timeout: 25_000
  });
  await expect(page.getByText("Decision Brief").first()).toBeVisible();
  await expect(page.getByText("Staged Brainstorming Room vertical slice").first()).toBeVisible();

  await page.getByRole("button", { name: "Accept Decision" }).click();
  await expect(page.getByText("decision_record.created").first()).toBeVisible({
    timeout: 25_000
  });
  await expect(page.getByText("Decision Record").first()).toBeVisible();

  await page.getByRole("button", { name: "Convert to Work Package" }).click();
  await expect(page.getByText("work_package.conversion_completed").first()).toBeVisible({
    timeout: 25_000
  });
  await expect(page.getByText("converted").first()).toBeVisible();
  await expect(page.getByText(/Work Package wp_/).first()).toBeVisible();
});
