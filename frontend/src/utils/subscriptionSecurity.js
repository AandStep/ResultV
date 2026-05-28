/*
 * Matches app.go ErrInsecureSubscription (Wails surfaces it as err.message).
 */
export const INSECURE_SUBSCRIPTION_ERROR_MARKER =
  "subscription URL uses plaintext HTTP";

export function isInsecureSubscriptionError(err) {
  const msg = String(err?.message ?? err ?? "");
  return msg.includes(INSECURE_SUBSCRIPTION_ERROR_MARKER);
}
