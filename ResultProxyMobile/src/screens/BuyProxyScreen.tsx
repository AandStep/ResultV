import React, { useCallback, useState } from 'react';
import {
    View,
    Text,
    Pressable,
    Linking,
    StyleSheet,
    ScrollView,
} from 'react-native';
import Clipboard from '@react-native-clipboard/clipboard';
import { ShoppingCart, Copy, ExternalLink, Check } from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import { useLogStore } from '../store/logStore';
import { colors } from '../theme';

const AFFILIATE_LINK = 'https://proxy6.net/?r=833290';
const PROMO_CODE = 'resultproxy';

export const BuyProxyScreen = () => {
    const { t } = useTranslation();
    const addLog = useLogStore(s => s.addLog);

    const [linkCopied, setLinkCopied] = useState(false);
    const [promoCopied, setPromoCopied] = useState(false);

    const handleCopyAndGo = useCallback(() => {
        Clipboard.setString(AFFILIATE_LINK);
        addLog('Ссылка скопирована.', 'success');

        setLinkCopied(true);
        setTimeout(() => setLinkCopied(false), 2000);

        Linking.openURL(AFFILIATE_LINK);
    }, [addLog]);

    const handleCopyPromo = useCallback(() => {
        Clipboard.setString(PROMO_CODE);
        addLog('Промокод скопирован.', 'success');

        setPromoCopied(true);
        setTimeout(() => setPromoCopied(false), 2000);
    }, [addLog]);

    return (
        <ScrollView style={styles.scrollView} contentContainerStyle={styles.container}>
            <View style={styles.headerSection}>
                <Text style={styles.title}>{t('buy.title')}</Text>
                <Text style={styles.desc}>{t('buy.desc')}</Text>
            </View>

            <View style={styles.card}>
                <View style={styles.iconContainer}>
                    <ShoppingCart size={32} color={colors.primary} />
                </View>
                <Text style={styles.cardTitle}>{t('buy.discount')}</Text>
                <Text style={styles.cardDesc}>{t('buy.discount_desc')}</Text>

                <View style={styles.actionList}>
                    <Pressable
                        onPress={handleCopyAndGo}
                        style={({ pressed }) => [styles.actionBtn, pressed && styles.actionBtnPressed]}
                    >
                        <View style={styles.actionTopRow}>
                            <View style={styles.actionIconBox}>
                                <ExternalLink size={20} color={colors.textSecondary} />
                            </View>
                            <Text style={styles.actionText} numberOfLines={1}>
                                {AFFILIATE_LINK}
                            </Text>
                        </View>
                        <View style={styles.actionBottomBtn}>
                            {linkCopied ? (
                                <>
                                    <Check size={16} color={colors.primaryLight} />
                                    <Text style={[styles.actionBottomBtnText, styles.actionBottomBtnTextSuccess]}>
                                        {t('buy.copied')}
                                    </Text>
                                </>
                            ) : (
                                <>
                                    <Copy size={16} color={colors.textSecondary} />
                                    <Text style={styles.actionBottomBtnText}>
                                        {t('buy.go')}
                                    </Text>
                                </>
                            )}
                        </View>
                    </Pressable>

                    <Pressable
                        onPress={handleCopyPromo}
                        style={({ pressed }) => [styles.actionBtn, pressed && styles.actionBtnPressed]}
                    >
                        <View style={styles.actionTopRow}>
                            <View style={styles.actionIconBox}>
                                <Copy size={20} color={colors.textSecondary} />
                            </View>
                            <View style={styles.promoTextContainer}>
                                <Text style={styles.actionTextPromoTitle}>
                                    {t('buy.promo_title')}
                                </Text>
                                <Text style={styles.actionTextPromoCode} numberOfLines={1}>
                                    {PROMO_CODE}
                                </Text>
                            </View>
                        </View>
                        <View style={styles.actionBottomBtn}>
                            {promoCopied ? (
                                <>
                                    <Check size={16} color={colors.primaryLight} />
                                    <Text style={[styles.actionBottomBtnText, styles.actionBottomBtnTextSuccess]}>
                                        {t('buy.copied')}
                                    </Text>
                                </>
                            ) : (
                                <>
                                    <Copy size={16} color={colors.textSecondary} />
                                    <Text style={styles.actionBottomBtnText}>
                                        {t('buy.copy')}
                                    </Text>
                                </>
                            )}
                        </View>
                    </Pressable>
                </View>
            </View>
        </ScrollView>
    );
};

const styles = StyleSheet.create({
    scrollView: { flex: 1, backgroundColor: colors.bg },
    container: { padding: 16, gap: 24, paddingBottom: 32 },
    headerSection: {},
    title: { fontSize: 30, lineHeight: 36, fontWeight: '700', color: colors.text },
    desc: { fontSize: 16, lineHeight: 24, color: '#a1a1aa', marginTop: 8 },

    card: {
        backgroundColor: colors.card,
        padding: 32,
        borderRadius: 24,
        borderWidth: 1,
        borderColor: colors.border,
        alignItems: 'center',
    },
    iconContainer: {
        backgroundColor: 'rgba(0, 126, 58, 0.1)',
        width: 64,
        height: 64,
        borderRadius: 16,
        alignItems: 'center',
        justifyContent: 'center',
        marginBottom: 24,
    },
    cardTitle: {
        fontSize: 20,
        lineHeight: 28,
        fontWeight: '700',
        color: colors.text,
        marginBottom: 16,
        textAlign: 'center',
    },
    cardDesc: {
        fontSize: 16,
        lineHeight: 24,
        color: '#a1a1aa',
        marginBottom: 32,
        textAlign: 'center',
    },

    actionList: {
        width: '100%',
        gap: 16,
    },
    actionBtn: {
        width: '100%',
        backgroundColor: '#09090b', // zinc-950
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 16,
        padding: 16,
        gap: 16,
    },
    actionBtnPressed: {
        borderColor: 'rgba(0, 168, 25, 0.5)',
        backgroundColor: 'rgba(0, 168, 25, 0.02)',
    },
    actionTopRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 16,
    },
    actionIconBox: {
        backgroundColor: colors.card,
        padding: 12,
        borderRadius: 12,
        borderWidth: 1,
        borderColor: colors.border,
    },
    actionText: {
        color: '#d4d4d8', // zinc-300
        fontSize: 14,
        lineHeight: 20,
        fontFamily: 'monospace',
        flex: 1,
    },
    promoTextContainer: {
        flex: 1,
        justifyContent: 'center',
    },
    actionTextPromoTitle: {
        color: '#71717a', // zinc-500
        fontSize: 12,
        lineHeight: 16,
        fontWeight: '500',
        marginBottom: 2,
    },
    actionTextPromoCode: {
        color: '#d4d4d8', // zinc-300
        fontSize: 14,
        lineHeight: 20,
        fontWeight: '700',
        fontFamily: 'monospace',
        textTransform: 'uppercase',
        letterSpacing: 2,
    },
    actionBottomBtn: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'center',
        width: '100%',
        backgroundColor: colors.card,
        paddingVertical: 12,
        paddingHorizontal: 24,
        borderRadius: 12,
        borderWidth: 1,
        borderColor: colors.border,
        gap: 8,
    },
    actionBottomBtnText: {
        fontSize: 14,
        lineHeight: 20,
        fontWeight: '500',
        color: '#a1a1aa', // zinc-400
    },
    actionBottomBtnTextSuccess: {
        color: colors.primaryLight,
    },
});
