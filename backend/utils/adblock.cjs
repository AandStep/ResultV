const fs = require('fs');
const path = require('path');
const { FiltersEngine, Request } = require('@ghostery/adblocker');
const fetch = require('cross-fetch');

/**
 * Ad Blocker Module powered by @ghostery/adblocker
 */

// Резервный список на время загрузки движка
const FALLBACK_AD_DOMAINS = [
  "doubleclick.net",
  "googlesyndication.com",
  "googleadservices.com",
  "googletagmanager.com",
  "google-analytics.com",
  "yandexadexchange.net",
  "mc.yandex.ru",
  "ads.vk.com",
  "adcash.com",
  "adsterra.com",
  "popads.net"
];

let engine = null;
let isInitializing = false;

/**
 * Инициализирует движок Ghostery.
 * @param {string} userDataPath - Путь к папке данных приложения для кэша.
 */
async function initEngine(userDataPath) {
  if (engine || isInitializing) return;
  isInitializing = true;

  try {
    const cachePath = path.join(userDataPath, 'adblock_engine.dat');
    
    // Пытаемся загрузить из кэша
    if (fs.existsSync(cachePath)) {
      try {
        const buffer = fs.readFileSync(cachePath);
        engine = FiltersEngine.deserialize(buffer);
        console.log('[ADBLOCK] Движок восстановлен из локального кэша.');
      } catch (e) {
        console.error('[ADBLOCK] Ошибка десериализации кэша:', e.message);
      }
    }

    // Если движок не загружен из кэша или кэша нет, скачиваем списки
    if (!engine) {
      console.log('[ADBLOCK] Загрузка EasyList и EasyPrivacy от Ghostery...');
      // fromPrebuiltAdsAndTracking включает EasyList, EasyPrivacy, Peter Lowe's list и uBlock filters
      engine = await FiltersEngine.fromPrebuiltAdsAndTracking(fetch);
      console.log('[ADBLOCK] Движок успешно инициализирован.');

      // Сохраняем в кэш для следующего запуска
      try {
        const dir = path.dirname(cachePath);
        if (!fs.existsSync(dir)) {
          fs.mkdirSync(dir, { recursive: true });
        }
        const serialized = engine.serialize();
        fs.writeFileSync(cachePath, Buffer.from(serialized));
        console.log('[ADBLOCK] Движок сохранен в кэш.');
      } catch (e) {
        console.error('[ADBLOCK] Не удалось сохранить движок в кэш:', e.message);
      }
    }
  } catch (error) {
    console.error('[ADBLOCK] Критическая ошибка инициализации:', error.message);
  } finally {
    isInitializing = false;
  }
}

/**
 * Проверяет, является ли запрос рекламным.
 * 
 * @param {string} hostname - Хост (всегда есть)
 * @param {string} url - Полный URL (если есть, например в HTTP мосте)
 * @param {string} type - Тип запроса (script, image, xmlhttprequest и т.д.)
 * @returns {boolean}
 */
function isAdDomain(hostname, url = null, type = 'xmlhttprequest') {
  // 1. Если движок еще не загружен, используем упрощенный список
  if (!engine) {
    const h = (hostname || '').toLowerCase().trim();
    for (const domain of FALLBACK_AD_DOMAINS) {
      if (h === domain || h.endsWith('.' + domain)) return true;
    }
    return false;
  }

  // 2. Используем движок Ghostery
  try {
    // Если нет полного URL, конструируем фиктивный для проверки домена
    const testUrl = url || `http://${hostname}/`;
    const request = Request.fromRawDetails({
      url: testUrl,
      type: type,
      sourceUrl: 'http://localhost/' // Фиктивный источник
    });

    const match = engine.match(request);
    return match.match; // Используем свойство match объекта (boolean)
  } catch (e) {
    return false;
  }
}

module.exports = { initEngine, isAdDomain };
