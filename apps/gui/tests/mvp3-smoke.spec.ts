import { expect, test } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const projectRoot = process.env.ARTEMIS_E2E_PROJECT_ROOT ?? createTempProject("artemis-mvp3-");

function createTempProject(prefix: string) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  fs.writeFileSync(path.join(root, "README.md"), "Artemis MVP 3 GUI smoke project\n", "utf8");
  fs.mkdirSync(path.join(root, "tests"));
  fs.writeFileSync(
    path.join(root, "tests", "test_smoke.py"),
    "import unittest\n\nclass SmokeTest(unittest.TestCase):\n    def test_smoke(self):\n        self.assertTrue(True)\n",
    "utf8"
  );
  return root;
}

test("runs the MVP 3 implementation pipeline through patch apply and review", async ({ page }) => {
  const projectName = `Artemis MVP3 ${Date.now()}`;

  await page.goto("/");
  await expect(page.getByText("online")).toBeVisible();

  await page.getByLabel("Name").fill(projectName);
  await page.getByLabel("Root Path").fill(projectRoot);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(page.getByRole("button", { name: new RegExp(projectName) })).toBeVisible();

  await page.getByLabel("Title").fill("MVP 3 E2E Smoke");
  await page.getByRole("button", { name: "Create" }).click();

  await page
    .getByPlaceholder("Describe the work package.")
    .fill("Create an MVP 3 implementation pipeline smoke Work Package.");
  await page.getByRole("button", { name: "Submit" }).click();

  await expect(page.getByText("work_package.pending_approval")).toBeVisible({ timeout: 25_000 });
  await page.getByRole("button", { name: "Approve", exact: true }).click();
  await expect(page.getByText("approved").first()).toBeVisible();

  await page.getByRole("button", { name: "Start" }).click();
  await expect(page.getByText("patch_set.pending_approval").first()).toBeVisible({
    timeout: 25_000
  });
  await expect(page.getByText("docs/artemis_implementation_log.md").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Approve Patch" })).toBeEnabled();

  await page.getByRole("button", { name: "Approve Patch" }).click();
  await expect(page.getByText("approved").first()).toBeVisible();
  await page.getByRole("button", { name: "Apply", exact: true }).click();

  await expect(page.getByText("patch_set.applied").first()).toBeVisible({ timeout: 25_000 });
  await expect(page.getByText("verification.completed").first()).toBeVisible();
  await expect(page.getByText("review.completed").first()).toBeVisible();
  await expect(page.getByText("pass").first()).toBeVisible();
});
