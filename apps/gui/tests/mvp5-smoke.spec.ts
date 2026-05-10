import { expect, test } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const projectRoot = process.env.ARTEMIS_E2E_PROJECT_ROOT ?? createTempProject("artemis-mvp5-");

function createTempProject(prefix: string) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  fs.writeFileSync(path.join(root, "README.md"), "Artemis MVP 5 GUI smoke project\n", "utf8");
  fs.mkdirSync(path.join(root, "docs"));
  fs.writeFileSync(
    path.join(root, "docs", "artemis_mvp5.md"),
    "Memory and Decision Log\n",
    "utf8"
  );
  return root;
}

test("runs the MVP 5 memory decision log and selected context flow", async ({ page }) => {
  const projectName = `Artemis MVP5 ${Date.now()}`;

  await page.goto("/");
  await expect(page.getByText("online")).toBeVisible();

  await page.getByLabel("Name").fill(projectName);
  await page.getByLabel("Root Path").fill(projectRoot);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(page.getByRole("button", { name: new RegExp(projectName) })).toBeVisible();

  await page.getByLabel("Title").fill("MVP 5 E2E Smoke");
  await page.getByRole("button", { name: "Create" }).click();

  await page
    .getByPlaceholder("Frame the decision or design topic.")
    .fill("Decide the MVP 5 Memory and Decision Log scope.");
  await page.getByRole("button", { name: "Start Brainstorming" }).click();
  await expect(page.getByText("brainstorming.decision_brief_created")).toBeVisible({
    timeout: 25_000
  });

  await page.getByRole("button", { name: "Accept Decision" }).click();
  await expect(page.getByText("decision_record.created").first()).toBeVisible({
    timeout: 25_000
  });

  await page.getByRole("button", { name: "Promote Decision" }).click();
  await expect(page.getByText(/Decision:/).first()).toBeVisible({ timeout: 25_000 });
  await expect(page.getByText("decision").first()).toBeVisible();

  await page.getByRole("button", { name: "rules" }).click();
  await page.getByRole("button", { name: "Create Rule" }).click();
  await expect(page.getByText("GUI calls Control Plane only").first()).toBeVisible({
    timeout: 25_000
  });
  await expect(page.getByText(/manual:mem_/).first()).toBeVisible();

  await page.getByPlaceholder("Search memory").fill("Control Plane");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page.getByText("The GUI must not call Agent Backend directly.").first()).toBeVisible();

  await page.getByRole("button", { name: "Select Context" }).click();
  await expect(page.getByText("Selected Context").first()).toBeVisible();
  await expect(page.getByText("GUI calls Control Plane only").first()).toBeVisible();

  await page.getByTitle("Remove").click();
  await expect(page.locator(".selected-memory-strip").getByText("No selected memory")).toBeVisible();

  await page.getByRole("button", { name: "Archive" }).click();
  await expect(page.getByRole("button", { name: "Restore" })).toBeVisible();
  await page.getByRole("button", { name: "Restore" }).click();
  await expect(page.getByRole("button", { name: "Archive" })).toBeVisible();
});
