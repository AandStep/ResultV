import { wailsAPI } from "./wailsAPI";

export const isWindowsHost =
  typeof navigator !== "undefined" && /Windows/i.test(navigator.userAgent);

/* Окно «нужны права администратора»: вторая кнопка сразу включает запуск от
   администратора навсегда (только Windows). */
export function adminRestartDialog(t, proxyId = "", mode = "") {
  return {
    title: t("tunnel.adminTitle"),
    message: t("tunnel.adminMessage"),
    variant: "warning",
    confirmText: t("tunnel.restartAsAdmin"),
    onConfirmAction: () => wailsAPI.restartAsAdmin(proxyId, mode),
    ...(isWindowsHost
      ? {
          secondaryText: t("tunnel.alwaysAdmin"),
          onSecondaryAction: () => wailsAPI.enableAlwaysAdminAndRestart(proxyId, mode),
        }
      : {}),
  };
}
