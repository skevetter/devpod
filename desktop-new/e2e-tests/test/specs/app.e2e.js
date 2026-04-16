describe("App Launch", () => {
  it("should render the sidebar", async () => {
    // The sidebar is the first child of the flex container
    const sidebar = await $("nav")
    await expect(sidebar).toBeDisplayed()
  })

  it("should show the Dashboard heading", async () => {
    const heading = await $("h2")
    const text = await heading.getText()
    expect(text).toMatch(/dashboard/i)
  })

  it("should display workspace count on the dashboard", async () => {
    // Dashboard shows "Workspaces" stat card
    const body = await $("main")
    const text = await body.getText()
    expect(text).toContain("Workspaces")
  })
})
