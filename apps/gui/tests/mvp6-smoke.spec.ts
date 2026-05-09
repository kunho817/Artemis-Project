import { expect, test } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const projectRoot = process.env.ARTEMIS_E2E_PROJECT_ROOT ?? createTempProject("artemis-mvp6-");

function createTempProject(prefix: string) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  fs.writeFileSync(path.join(root, "README.md"), "Artemis MVP 6 GUI smoke project\n", "utf8");
  fs.mkdirSync(path.join(root, "docs"));
  fs.writeFileSync(
    path.join(root, "docs", "artemis_mvp6.md"),
    "Risk Radar and Quality Center\n",
    "utf8"
  );
  return root;
}

test("runs the MVP 6 Risk Radar and Quality Center flow", async ({ page }) => {
  const projectName = `Artemis MVP6 ${Date.now()}`;

  await page.goto("/");
  await expect(page.getByText("online")).toBeVisible();

  await page.getByLabel("Name").fill(projectName);
  await page.getByLabel("Root Path").fill(projectRoot);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(page.getByRole("button", { name: new RegExp(projectName) })).toBeVisible();

  await page.getByLabel("Title").fill("MVP 6 E2E Smoke");
  await page.getByRole("button", { name: "Create" }).click();

  await page.getByRole("button", { name: "rules" }).click();
  await page.getByRole("button", { name: "Create Rule" }).click();
  await expect(page.getByText("GUI calls Control Plane only").first()).toBeVisible({
    timeout: 25_000
  });
  await page.getByRole("button", { name: "Select Context" }).click();
  await expect(page.getByText("Include selected memory (1)")).toBeVisible();

  await page.getByRole("button", { name: "Start Risk Scan" }).click();
  await expect(page.getByText("risk_scan.completed").first()).toBeVisible({ timeout: 25_000 });
  await expect(page.getByText("Project Health")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Risk Findings" })).toBeVisible();
  await expect(page.getByText("Repository-level tests were not detected").first()).toBeVisible();
  await expect(page.getByText("Source Links").first()).toBeVisible();
  await expect(page.getByRole("heading", { name: "Quality Center" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Architecture Map Lite" })).toBeVisible();

  await page.getByRole("button", { name: "Accept Finding" }).click();
  await expect(page.getByText("risk_finding.accepted").first()).toBeVisible({ timeout: 25_000 });

  await page.getByRole("button", { name: "Convert to Work Package" }).click();
  await expect(page.getByText("risk_finding.converted_to_work_package").first()).toBeVisible({
    timeout: 25_000
  });
  await expect(page.getByText("pending_approval").first()).toBeVisible();
});
