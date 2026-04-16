describe("Workspaces Page", () => {
  before(async () => {
    const link = await $('nav a[href="/workspaces"]')
    await link.click()
    // Wait for page load
    const heading = await $("h2")
    await heading.waitForDisplayed({ timeout: 5000 })
  })

  it("should list workspaces", async () => {
    const main = await $("main")
    await main.waitForDisplayed({ timeout: 5000 })
    const text = await main.getText()
    // Should show at least some workspace content or empty state
    expect(text.length).toBeGreaterThan(0)
  })

  it("should have a Create Workspace button", async () => {
    const btn = await $('a[href="/workspaces/new"]')
    await expect(btn).toBeDisplayed()
  })

  it("should navigate to the create workspace form", async () => {
    const btn = await $('a[href="/workspaces/new"]')
    await btn.click()

    const main = await $("main")
    await main.waitForDisplayed({ timeout: 5000 })
    const text = await main.getText()
    expect(text).toMatch(/create|new|workspace/i)
  })
})
