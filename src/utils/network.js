import { apiFetch } from "../hooks/useLogs";

/**
 * Определяет страну по IP через бэкенд API.
 * Фронтенд НЕ обращается к внешним GEO-сервисам напрямую.
 */
export const detectCountry = async (ip) => {
  try {
    let cleanIp = ip.split(":")[0];
    if (
      cleanIp === "127.0.0.1" ||
      cleanIp === "localhost" ||
      cleanIp.startsWith("192.168.") ||
      cleanIp.startsWith("10.")
    ) {
      return "local";
    }

    const res = await apiFetch("/api/detect-country", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ip: cleanIp }),
    });

    if (res.ok) {
      const data = await res.json();
      if (data.country && data.country !== "🌐" && data.country !== "🏠") {
        return data.country;
      }
      if (data.country === "🏠") return "local";
    }
  } catch (error) {}

  return "unknown";
};
