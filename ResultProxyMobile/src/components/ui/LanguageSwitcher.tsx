import React, { memo, useCallback } from 'react';
import { Pressable, StyleSheet } from 'react-native';
import { useTranslation } from 'react-i18next';
import { FlagIcon } from './FlagIcon';
import { colors } from '../../theme';

export const LanguageSwitcher = memo(() => {
    const { i18n } = useTranslation();

    const toggleLanguage = useCallback(() => {
        const nextLang = i18n.language?.startsWith('ru') ? 'en' : 'ru';
        i18n.changeLanguage(nextLang);
    }, [i18n]);

    const isRu = i18n.language?.startsWith('ru');

    return (
        <Pressable
            onPress={toggleLanguage}
            style={styles.button}
            android_ripple={{ color: colors.border, borderless: true }}>
            <FlagIcon code={isRu ? 'RU' : 'US'} size={20} />
        </Pressable>
    );
});

LanguageSwitcher.displayName = 'LanguageSwitcher';

const styles = StyleSheet.create({
    button: {
        padding: 8,
        borderRadius: 12,
    },
});
