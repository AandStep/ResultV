export const detectCountry = async (ip, name = "") => {
  try {
    let cleanIp = ip.split(":")[0];
    if (
      cleanIp === "127.0.0.1" ||
      cleanIp === "localhost" ||
      cleanIp.startsWith("192.168.") ||
      cleanIp.startsWith("10.") // Local network check
    ) {
      return "local";
    }

    const controller = new AbortController();
    // Increase timeout a bit to account for multiple APIs
    const id = setTimeout(() => controller.abort(), 4000);

    let countryCode = null;

    // 1. Try api.iplocation.net first (Very accurate for physical location vs ASN)
    try {
      const res = await fetch(`https://api.iplocation.net/?ip=${cleanIp}`, {
        signal: controller.signal,
      });
      if (res.ok) {
        const data = await res.json();
        if (data && data.country_code2 && data.country_code2 !== "-") {
          countryCode = data.country_code2;
        }
      }
    } catch (e) {}

    // 2. Try api.ip2location.io (Excellent fallback for physical accuracy)
    if (!countryCode) {
      try {
        const res3 = await fetch(`https://api.ip2location.io/?ip=${cleanIp}`, {
          signal: controller.signal,
        });
        if (res3.ok) {
          const data3 = await res3.json();
          if (data3 && data3.country_code && data3.country_code !== "-") {
            countryCode = data3.country_code;
          }
        }
      } catch (e) {}
    }

    // 3. Fall back to ip-api.com (handles domains well, but HTTP so might be blocked)
    if (!countryCode) {
      try {
        const res2 = await fetch(
          `http://ip-api.com/json/${cleanIp}?fields=countryCode`,
          { signal: controller.signal },
        );
        if (res2.ok) {
          const data2 = await res2.json();
          if (data2 && data2.countryCode) {
            countryCode = data2.countryCode;
          }
        }
      } catch (e) {
        // Ignore error
      }
    }

    clearTimeout(id);

    if (countryCode) {
      return countryCode.toLowerCase();
    }
  } catch (error) {}

  const s = name.toLowerCase();
  if (
    s.includes("ru") ||
    s.includes("rus") ||
    s.includes("ру") ||
    s.includes("россия")
  )
    return "ru";
  if (
    s.includes("us") ||
    s.includes("usa") ||
    s.includes("сша") ||
    s.includes("america")
  )
    return "us";
  if (s.includes("de") || s.includes("germany") || s.includes("герм"))
    return "de";
  if (
    s.includes("uk") ||
    s.includes("gb") ||
    s.includes("london") ||
    s.includes("англия") ||
    s.includes("британ")
  )
    return "gb";
  if (
    s.includes("nl") ||
    s.includes("neth") ||
    s.includes("нидерланд") ||
    s.includes("голландия")
  )
    return "nl";
  if (s.includes("fr") || s.includes("france") || s.includes("франц"))
    return "fr";
  if (s.includes("kz") || s.includes("kazakhstan") || s.includes("казах"))
    return "kz";
  if (s.includes("ua") || s.includes("ukraine") || s.includes("украин"))
    return "ua";
  if (s.includes("tr") || s.includes("turkey") || s.includes("турц"))
    return "tr";
  if (s.includes("fi") || s.includes("finland") || s.includes("финлянд"))
    return "fi";

  return "unknown";
};
