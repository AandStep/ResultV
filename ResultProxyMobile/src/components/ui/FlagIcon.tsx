import React, { useState, memo, useCallback } from 'react';
import { Image, StyleSheet, Text } from 'react-native';
import { Globe, Server } from 'lucide-react-native';
import { colors } from '../../theme';

type Props = {
    code: string;
    size?: number;
};

const FLAG_CDN =
    'https://cdnjs.cloudflare.com/ajax/libs/flag-icons/7.2.3/flags/4x3';

const getIsoCode = (code: string): string | null => {
    if (/^[a-zA-Z]{2}$/.test(code)) return code.toLowerCase();

    const clean = code.replace(/[\uFE0F]/g, '').trim();
    if (clean.length > 0) {
        const cp1 = clean.codePointAt(0);
        if (cp1 && cp1 >= 0x1f1e6 && cp1 <= 0x1f1ff) {
            const cp2 = clean.codePointAt(2);
            if (cp2 && cp2 >= 0x1f1e6 && cp2 <= 0x1f1ff) {
                return (
                    String.fromCharCode(cp1 - 0x1f1e6 + 97) +
                    String.fromCharCode(cp2 - 0x1f1e6 + 97)
                );
            }
        }
    }
    return null;
};

export const FlagIcon = memo(({ code, size = 24 }: Props) => {
    const [imgError, setImgError] = useState(false);
    const onError = useCallback(() => setImgError(true), []);

    if (!code || code === 'unknown' || code === '🌐') {
        return <Globe size={size} color={colors.textMuted} />;
    }
    if (code === 'local' || code === '🏠') {
        return <Server size={size} color={colors.textMuted} />;
    }

    const isoCode = getIsoCode(code);

    if (isoCode && !imgError) {
        return (
            <Image
                source={{ uri: `${FLAG_CDN}/${isoCode}.svg` }}
                style={[styles.flag, { width: size, height: size * 0.75 }]}
                onError={onError}
            />
        );
    }

    return (
        <Text style={styles.fallback}>{isoCode?.toUpperCase() || code}</Text>
    );
});

FlagIcon.displayName = 'FlagIcon';

const styles = StyleSheet.create({
    flag: {
        borderRadius: 2,
    },
    fallback: {
        fontSize: 12,
        fontWeight: '700',
        color: colors.textSecondary,
        textTransform: 'uppercase',
        letterSpacing: 1,
    },
});
