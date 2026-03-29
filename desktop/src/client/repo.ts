export const REPO_OWNER = "skevetter"
export const REPO_NAME = "devpod"
export const REPO_SLUG = `${REPO_OWNER}/${REPO_NAME}`
export const BINARY_NAME = REPO_NAME
export const PRODUCT_NAME = "DevPod"
export const PRODUCT_NAME_PRO = `${PRODUCT_NAME} Pro`
export const PRO_RELEASE_NAME = `${REPO_NAME}-pro`
export const PROVIDER_PREFIX = `${REPO_NAME}-provider-`
export const SSH_HOST_SUFFIX = `.${BINARY_NAME}`

export const GITHUB_REPO_URL = `https://github.com/${REPO_SLUG}`
export const GITHUB_RELEASES_URL = `${GITHUB_REPO_URL}/releases`
export const GITHUB_ISSUES_URL = `${GITHUB_REPO_URL}/issues/new/choose`
export const WEBSITE_BASE_URL = `https://${REPO_NAME}.sh`
export const WEBSITE_DOCS_URL = `${WEBSITE_BASE_URL}/docs`
export const WEBSITE_PRO_URL = `${WEBSITE_BASE_URL}/pro`
export const WEBSITE_ASSETS_URL = `${WEBSITE_BASE_URL}/assets`

export const FLATPAK_ID = `sh.loft.${REPO_NAME}`
export const CONTAINER_NAME = BINARY_NAME
export const SHARED_MACHINE_PREFIX = `${BINARY_NAME}-shared-`
