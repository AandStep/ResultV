import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { NativeModules, Platform } from 'react-native';

import en from '../locales/en.json';
import ru from '../locales/ru.json';

const getDeviceLanguage = (): string => {
    const locale =
        Platform.OS === 'android'
            ? NativeModules.I18nManager?.localeIdentifier
            : NativeModules.SettingsManager?.settings?.AppleLocale ||
            NativeModules.SettingsManager?.settings?.AppleLanguages?.[0];
    return locale?.startsWith('ru') ? 'ru' : 'en';
};

const languageDetector = {
    type: 'languageDetector' as const,
    async: true,
    detect: async (callback: (lang: string) => void) => {
        const saved = await AsyncStorage.getItem('language');
        callback(saved || getDeviceLanguage());
    },
    init: () => { },
    cacheUserLanguage: async (lang: string) => {
        await AsyncStorage.setItem('language', lang);
    },
};

i18n
    .use(languageDetector)
    .use(initReactI18next)
    .init({
        resources: {
            en: { translation: en },
            ru: { translation: ru },
        },
        fallbackLng: 'en',
        interpolation: {
            escapeValue: false,
        },
    });

export default i18n;
