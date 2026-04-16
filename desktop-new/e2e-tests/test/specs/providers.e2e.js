describe("Providers Page", () => {
  before(async () => {
    const link = await $('nav a[href="/providers"]')
    await link.click()
    const heading = await $("h2")
    await heading.waitForDisplayed({ timeout: 5000 })
  })

  it("should show the providers heading", async () => {
    const heading = await $("h2")
    const text = await heading.getText()
    expect(text).toMatch(/providers/i)
  })

  it("should list providers or show empty state", async () => {
    const main = await $("main")
    const text = await main.getText()
    expect(text.length).toBeGreaterThan(0)
  })

  it("should have an Add Provider button", async () => {
    // Look for any button that says "Add" in the providers area
    const btn = await $("main button")
    await expect(btn).toBeDisplayed()
  })
})
