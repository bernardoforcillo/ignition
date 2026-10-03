/**
 * Origin the email images are served from (`<origin>/static/...`). Unset in `pnpm dev`, where the
 * preview server serves `src/emails/static` itself. The Go export sets it to the `{{.AssetBaseUrl}}`
 * placeholder, so the committed HTML carries no environment-specific host.
 */
export const assetBaseUrl = process.env.EMAIL_ASSET_BASE_URL ?? "";
