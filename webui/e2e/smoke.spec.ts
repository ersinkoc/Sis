import { expect, test } from "@playwright/test";

test("first-run setup opens dashboard and runs a blocked query", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Create Admin" })).toBeVisible();
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("change-me-now");
  await page.getByRole("button", { name: "Create" }).click();

  await expect(page.getByText("admin").first()).toBeVisible();
  await expect(page.getByRole("heading", { name: "Live Summary" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "System" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Query Test" })).toBeVisible();

  const queryTest = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Query Test" }) });
  await queryTest.getByRole("textbox", { name: "Domain" }).fill("blocked.example.com");
  await queryTest.getByRole("button", { name: "Run" }).click();

  await expect(page.getByText("NOERROR")).toBeVisible();
  await expect(page.getByText("synthetic")).toBeVisible();
  await expect(page.getByText(/0\.0\.0\.0/)).toBeVisible();
});
