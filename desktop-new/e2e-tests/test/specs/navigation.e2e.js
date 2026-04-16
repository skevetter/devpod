describe("Sidebar Navigation", () => {
  it("should navigate to Workspaces page", async () => {
    const link = await $('nav a[href="/workspaces"]')
    await link.click()

    // Wait for the page to render
    const heading = await $("h2")
    await heading.waitForDisplayed({ timeout: 5000 })
    const text = await heading.getText()
    expect(text).toMatch(/workspaces/i)
  })

  it("should navigate to Providers page", async () => {
    const link = await $('nav a[href="/providers"]')
    await link.click()

    const heading = await $("h2")
    await heading.waitForDisplayed({ timeout: 5000 })
    const text = await heading.getText()
    expect(text).toMatch(/providers/i)
  })

  it("should navigate to Machines page", async () => {
    const link = await $('nav a[href="/machines"]')
    await link.click()

    const heading = await $("h2")
    await heading.waitForDisplayed({ timeout: 5000 })
    const text = await heading.getText()
    expect(text).toMatch(/machines/i)
  })

  it("should navigate back to Dashboard", async () => {
    const link = await $('nav a[href="/"]')
    await link.click()

    const heading = await $("h2")
    await heading.waitForDisplayed({ timeout: 5000 })
    const text = await heading.getText()
    expect(text).toMatch(/dashboard/i)
  })
})
