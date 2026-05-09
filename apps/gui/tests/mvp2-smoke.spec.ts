import { expect, test } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const projectRoot = process.env.ARTEMIS_E2E_PROJECT_ROOT ?? createTempProject("artemis-mvp2-");

function createTempProject(prefix: string) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  fs.writeFileSync(path.join(root, "README.md"), "Artemis GUI smoke project\n", "utf8");
  fs.mkdirSync(path.join(root, "tests"));
  fs.writeFileSync(
    path.join(root, "tests", "test_smoke.py"),
    "import unittest\n\nclass SmokeTest(unittest.TestCase):\n    def test_smoke(self):\n        self.assertTrue(True)\n",
    "utf8"
  );
  return root;
}

test("creates a work package and resolves approval through the GUI", async ({ page }) => {
  const projectName = `Artemis E2E ${Date.now()}`;

  await page.goto("/");
  await expect(page.getByText("Project Command Center")).toBeVisible();
  await expect(page.getByText("online")).toBeVisible();

  await page.getByLabel("Name").fill(projectName);
  await page.getByLabel("Root Path").fill(projectRoot);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(page.getByRole("button", { name: new RegExp(projectName) })).toBeVisible();

  await page.getByLabel("Title").fill("MVP 2 E2E Smoke");
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.getByRole("button", { name: /MVP 2 E2E Smoke/ })).toBeVisible();

  await page
    .getByPlaceholder("Describe the work package.")
    .fill("Validate the MVP 2 GUI event stream and approval controls.");
  await page.getByRole("button", { name: "Submit" }).click();

  await expect(page.getByText("agent_run.created")).toBeVisible();
  await expect(page.getByText("work_package.pending_approval")).toBeVisible({ timeout: 25_000 });
  await expect(page.getByText("Work Package").first()).toBeVisible();
  await expect(page.getByText("Approval").first()).toBeVisible();
  await expect(page.getByText("pending").first()).toBeVisible();

  await page.getByRole("button", { name: "Trace" }).click();
  await expect(page.getByText(/^trace_/)).toBeVisible();

  await page.getByRole("button", { name: "Artifacts" }).click();
  await expect(page.getByText("Intent Result")).toBeVisible();
  await expect(page.getByText("Context Summary")).toBeVisible();

  await page.getByRole("button", { name: "Approve", exact: true }).click();
  await expect(page.getByText("approved").first()).toBeVisible();
});

test("rejects a work package approval through the GUI", async ({ page }) => {
  const projectName = `Artemis Reject ${Date.now()}`;

  await page.goto("/");
  await expect(page.getByText("online")).toBeVisible();

  await page.getByLabel("Name").fill(projectName);
  await page.getByLabel("Root Path").fill(projectRoot);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(page.getByRole("button", { name: new RegExp(projectName) })).toBeVisible();

  await page.getByLabel("Title").fill("MVP 2 Reject Smoke");
  await page.getByRole("button", { name: "Create" }).click();

  await page
    .getByPlaceholder("Describe the work package.")
    .fill("Validate the GUI rejection path for Work Package approval.");
  await page.getByRole("button", { name: "Submit" }).click();
  await expect(page.getByText("work_package.pending_approval")).toBeVisible({ timeout: 25_000 });

  await page.getByRole("button", { name: "Reject", exact: true }).click();
  await expect(page.getByText("rejected").first()).toBeVisible();
});
