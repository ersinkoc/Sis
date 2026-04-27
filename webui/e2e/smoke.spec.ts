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
    .locator("form")
    .filter({ has: page.getByRole("heading", { name: "Query Test" }) });
  await queryTest.getByRole("textbox", { name: "Domain", exact: true }).fill("blocked.example.com");
  const queryResponsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/query/test") && response.request().method() === "POST",
  );
  await queryTest.getByRole("button", { name: "Run" }).click();
  const queryResponse = await queryResponsePromise;
  expect(queryResponse.ok()).toBeTruthy();
  const queryResult = (await queryResponse.json()) as { answers: string[]; rcode: string };

  expect(queryResult.rcode).toBe("NOERROR");
  expect(queryResult.answers.join("\n")).toContain("0.0.0.0");
});
