export const detectCountry = async (ip) => {
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

    let countryCode = null;

    // Функция для fetch с таймаутом в 2 сек (чтобы не тормозить UI)
    const fetchWithTimeout = async (url, ms = 2000) => {
      const controller = new AbortController();
      const id = setTimeout(() => controller.abort(), ms);
      const res = await fetch(url, { signal: controller.signal });
      clearTimeout(id);
      return res;
    };

    // 1. iplocation.net (Определяет физическое местоположение, очень точно для прокси)
    try {
      const res = await fetchWithTimeout(
        `https://api.iplocation.net/?ip=${cleanIp}`,
      );
      if (res.ok) {
        const data = await res.json();
        if (
          data &&
          data.country_code2 &&
          data.country_code2 !== "-" &&
          data.country_code2.length === 2
        ) {
          countryCode = data.country_code2;
        }
      }
    } catch (e) {}

    // 2. ip-api.com (Широко разрешен, хороший резерв)
    if (!countryCode) {
      try {
        const res2 = await fetchWithTimeout(
          `http://ip-api.com/json/${cleanIp}?fields=countryCode`,
        );
        if (res2.ok) {
          const data2 = await res2.json();
          if (data2 && data2.countryCode && data2.countryCode.length === 2) {
            countryCode = data2.countryCode;
          }
        }
      } catch (e) {}
    }

    // 3. GeoJS (Работает почти всегда, быстрый HTTPS, берем правильное поле)
    if (!countryCode) {
      try {
        const res3 = await fetchWithTimeout(
          `https://get.geojs.io/v1/ip/country/${cleanIp}.json`,
        );
        if (res3.ok) {
          const data3 = await res3.json();
          // Важно: берем country_code, а НЕ country (так как нужен формат "US", а не "United States")
          if (data3 && data3.country_code && data3.country_code.length === 2) {
            countryCode = data3.country_code;
          }
        }
      } catch (e) {}
    }

    // 4. Country.is (Надежная защита от падения предыдущих)
    if (!countryCode) {
      try {
        const res4 = await fetchWithTimeout(
          `https://api.country.is/${cleanIp}`,
        );
        if (res4.ok) {
          const data4 = await res4.json();
          if (data4 && data4.country && data4.country.length === 2) {
            countryCode = data4.country;
          }
        }
      } catch (e) {}
    }

    if (countryCode) {
      return countryCode.toLowerCase();
    }
  } catch (error) {}

  return "unknown";
};
