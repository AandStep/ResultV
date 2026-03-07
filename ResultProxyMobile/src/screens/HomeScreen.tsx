import React, { useCallback, useEffect, useState } from 'react';
import {
    View,
    Text,
    Pressable,
    ScrollView,
    StyleSheet,
} from 'react-native';
import {
    Power,
    Plus,
    Globe,
    Pencil,
    ChevronDown,
    Activity,
} from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import { FlagIcon } from '../components/ui/FlagIcon';
import { SpeedChart } from '../components/ui/SpeedChart';
import { useConfigStore } from '../store/configStore';
import { useConnectionStore } from '../store/connectionStore';
import { useLogStore } from '../store/logStore';
import { formatBytes, formatSpeed } from '../utils/formatters';
import { colors } from '../theme';

export const HomeScreen = ({ navigation }: any) => {
    const { t } = useTranslation();
    const proxies = useConfigStore(s => s.proxies);
    const setEditingProxy = useConfigStore(s => s.setEditingProxy);
    const routingRules = useConfigStore(s => s.routingRules);
    const settings = useConfigStore(s => s.settings);

    const isConnected = useConnectionStore(s => s.isConnected);
    const isProxyDead = useConnectionStore(s => s.isProxyDead);
    const failedProxy = useConnectionStore(s => s.failedProxy);
    const setFailedProxy = useConnectionStore(s => s.setFailedProxy);
    const activeProxy = useConnectionStore(s => s.activeProxy);
    const stats = useConnectionStore(s => s.stats);
    const speedHistory = useConnectionStore(s => s.speedHistory);
    const pings = useConnectionStore(s => s.pings);
    const toggleConnection = useConnectionStore(s => s.toggleConnection);
    const selectAndConnect = useConnectionStore(s => s.selectAndConnect);
    const addLog = useLogStore(s => s.addLog);

    const isError = !!failedProxy;
    const [isProxyListOpen, setIsProxyListOpen] = useState(false);
    const hasProxies = proxies.length > 0;
    const displayProxy = failedProxy || activeProxy || proxies[0];

    const handleToggle = useCallback(() => {
        if (isError) {
            setFailedProxy(null);
        }
        toggleConnection(proxies, routingRules, settings.killswitch, addLog);
    }, [isError, setFailedProxy, toggleConnection, proxies, routingRules, settings.killswitch, addLog]);

    const handleSelectProxy = useCallback(
        (proxy: any) => {
            selectAndConnect(proxy, routingRules, settings.killswitch, addLog);
            setIsProxyListOpen(false);
        },
        [selectAndConnect, routingRules, settings.killswitch, addLog],
    );

    const goToAdd = useCallback(() => {
        setEditingProxy(null);
        navigation.navigate('AddProxy');
    }, [setEditingProxy, navigation]);

    const onEditProxy = useCallback(
        (proxy: any) => {
            setEditingProxy(proxy);
            navigation.navigate('AddProxy');
        },
        [setEditingProxy, navigation],
    );

    const powerBtnColor = isConnected
        ? isProxyDead
            ? colors.error
            : colors.primary
        : isError
            ? colors.card
            : colors.card;

    const statusText = isConnected
        ? isProxyDead
            ? t('home.status.lost')
            : t('home.status.protected')
        : isError
            ? t('home.status.error')
            : t('home.status.unprotected');

    const statusColor = isConnected
        ? isProxyDead
            ? colors.error
            : colors.primary
        : isError
            ? colors.error
            : colors.textSecondary;

    const descText = isConnected
        ? isProxyDead
            ? t('home.desc.lost')
            : t('home.desc.protected')
        : isError
            ? t('home.desc.error')
            : t('home.desc.unprotected');

    const groupedProxies = Object.entries(
        proxies.reduce((acc: Record<string, any[]>, proxy) => {
            const cc = proxy.country || 'Unknown';
            if (!acc[cc]) acc[cc] = [];
            acc[cc].push(proxy);
            return acc;
        }, {}),
    ).sort(([a], [b]) => a.localeCompare(b));

    return (
        <ScrollView
            style={styles.scrollView}
            contentContainerStyle={styles.container}>
            <View style={styles.statusSection}>
                <Text style={[styles.statusTitle, { color: statusColor }]}>
                    {statusText}
                </Text>
                <Text style={styles.statusDesc}>{descText}</Text>
            </View>

            <View style={styles.powerSection}>
                {isConnected && !isProxyDead && (
                    <View style={[styles.powerGlow, { backgroundColor: colors.primary + '40' }]} />
                )}
                {(isProxyDead || isError) && (
                    <View style={[styles.powerGlow, { backgroundColor: colors.error + '30' }]} />
                )}
                <Pressable
                    disabled={!hasProxies && !isConnected}
                    onPress={handleToggle}
                    style={[
                        styles.powerBtn,
                        {
                            backgroundColor: powerBtnColor,
                            borderColor: isError
                                ? colors.error + '80'
                                : isConnected
                                    ? 'transparent'
                                    : colors.border,
                            opacity: !hasProxies && !isConnected ? 0.5 : 1,
                        },
                    ]}>
                    <Power
                        size={64}
                        color={
                            isConnected && !isProxyDead
                                ? colors.bg
                                : isError
                                    ? colors.error
                                    : colors.textSecondary
                        }
                    />
                </Pressable>
            </View>

            {!hasProxies ? (
                <View style={styles.emptySection}>
                    <Text style={styles.emptyText}>
                        {t('home.noProxies')}
                        <Text onPress={() => navigation.navigate('Buy')} style={{ color: colors.primaryLight, textDecorationLine: 'underline' }}>
                            {t('home.buyDiscount')}
                        </Text>
                    </Text>
                    <Pressable
                        onPress={goToAdd}
                        style={styles.addCard}
                        android_ripple={{ color: colors.border }}>
                        <View style={styles.addIconWrap}>
                            <Plus size={32} color={colors.textSecondary} />
                        </View>
                        <Text style={styles.addTitle}>{t('home.addServer')}</Text>
                        <Text style={styles.addDesc}>{t('home.addManual')}</Text>
                    </Pressable>
                </View>
            ) : (
                <View
                    style={[
                        styles.proxyCard,
                        {
                            borderColor:
                                (isProxyDead && isConnected) || isError
                                    ? colors.error + '50'
                                    : isProxyListOpen
                                        ? colors.borderLight
                                        : colors.border,
                        },
                    ]}>
                    <Pressable
                        onPress={() => setIsProxyListOpen(!isProxyListOpen)}
                        style={styles.proxyHeader}
                        android_ripple={{ color: colors.border }}>
                        <View style={styles.proxyHeaderLeft}>
                            <View
                                style={[
                                    styles.proxyIconWrap,
                                    {
                                        backgroundColor: isConnected
                                            ? isProxyDead
                                                ? colors.error + '20'
                                                : colors.primary + '20'
                                            : isError
                                                ? colors.error + '10'
                                                : colors.border,
                                    },
                                ]}>
                                {displayProxy ? (
                                    <FlagIcon code={displayProxy.country} size={28} />
                                ) : (
                                    <Globe size={28} color={colors.textMuted} />
                                )}
                            </View>
                            <View style={styles.proxyInfo}>
                                <Text style={styles.proxyLabel}>{t('home.currentServer')}</Text>
                                <Text style={styles.proxyName} numberOfLines={1}>
                                    {displayProxy ? displayProxy.name : t('home.emptyServer')}
                                </Text>
                                {displayProxy && (
                                    <Text style={styles.proxyAddr} numberOfLines={1}>
                                        {displayProxy.ip}:{displayProxy.port} ({displayProxy.type})
                                    </Text>
                                )}
                            </View>
                        </View>
                        <View style={styles.proxyActions}>
                            <Pressable
                                onPress={() => displayProxy && onEditProxy(displayProxy)}
                                hitSlop={8}
                                style={styles.editBtn}>
                                <Pencil size={18} color={colors.textDark} />
                            </Pressable>
                            <ChevronDown
                                size={18}
                                color={colors.textMuted}
                                style={isProxyListOpen ? { transform: [{ rotate: '180deg' }] } : undefined}
                            />
                        </View>
                    </Pressable>

                    {isProxyListOpen && (
                        <View style={styles.proxyDropdown}>
                            <ScrollView style={styles.proxyScrollList} nestedScrollEnabled>
                                {groupedProxies.map(([country, countryProxies]) => (
                                    <View key={country} style={styles.countryGroup}>
                                        <View style={styles.countryHeader}>
                                            <FlagIcon code={country} size={18} />
                                            <Text style={styles.countryLabel}>
                                                {country.toUpperCase()}
                                            </Text>
                                        </View>
                                        {(countryProxies as any[]).map((proxy: any) => {
                                            const isActive = activeProxy?.id === proxy.id;
                                            return (
                                                <Pressable
                                                    key={proxy.id}
                                                    onPress={() => handleSelectProxy(proxy)}
                                                    style={[
                                                        styles.proxyListItem,
                                                        isActive && styles.proxyListItemActive,
                                                    ]}
                                                    android_ripple={{ color: colors.border }}>
                                                    <View style={styles.proxyListItemLeft}>
                                                        <View style={styles.proxyListFlag}>
                                                            <FlagIcon code={proxy.country} size={20} />
                                                        </View>
                                                        <View>
                                                            <Text
                                                                style={[
                                                                    styles.proxyListName,
                                                                    isActive && { color: colors.primaryLight },
                                                                ]}
                                                                numberOfLines={1}>
                                                                {proxy.name}
                                                            </Text>
                                                            <Text style={styles.proxyListAddr} numberOfLines={1}>
                                                                {proxy.ip}:{proxy.port}
                                                            </Text>
                                                        </View>
                                                    </View>
                                                    <View style={styles.proxyListRight}>
                                                        <View style={styles.pingWrap}>
                                                            <Activity size={12} color={colors.textMuted} />
                                                            <Text style={styles.pingText}>
                                                                {pings[proxy.id] || '...'}
                                                            </Text>
                                                        </View>
                                                        <View
                                                            style={[
                                                                styles.dot,
                                                                {
                                                                    backgroundColor: isActive
                                                                        ? colors.primaryLight
                                                                        : colors.borderLight,
                                                                },
                                                            ]}
                                                        />
                                                    </View>
                                                </Pressable>
                                            );
                                        })}
                                    </View>
                                ))}
                            </ScrollView>
                            <Pressable
                                onPress={() => {
                                    setIsProxyListOpen(false);
                                    navigation.navigate('ProxyList');
                                }}
                                style={styles.openListBtn}
                                android_ripple={{ color: colors.border }}>
                                <Text style={styles.openListText}>{t('home.openList')}</Text>
                            </Pressable>
                        </View>
                    )}
                </View>
            )}

            {isError ? (
                <View style={styles.errorActions}>
                    <Pressable
                        onPress={() => displayProxy && onEditProxy(displayProxy)}
                        style={styles.errorBtn}
                        android_ripple={{ color: colors.border }}>
                        <Text style={styles.errorBtnText}>{t('home.editData')}</Text>
                    </Pressable>
                    <Pressable
                        onPress={() => {
                            setFailedProxy(null);
                            navigation.navigate('ProxyList');
                        }}
                        style={styles.errorBtnAlt}
                        android_ripple={{ color: colors.error + '30' }}>
                        <Text style={styles.errorBtnAltText}>
                            {t('home.chooseOther')}
                        </Text>
                    </Pressable>
                </View>
            ) : (
                isConnected && (
                    <View style={styles.statsRow}>
                        <View style={styles.statCard}>
                            <View style={styles.statHeader}>
                                <Text style={styles.statLabel}>{t('home.download')}</Text>
                                <Text
                                    style={[
                                        styles.statSpeed,
                                        { color: isProxyDead ? colors.textDark : colors.primary },
                                    ]}>
                                    {formatSpeed(speedHistory.down[19])}
                                </Text>
                            </View>
                            <Text
                                style={[
                                    styles.statValue,
                                    { color: isProxyDead ? colors.textDark : colors.primary },
                                ]}>
                                {formatBytes(stats.download)}
                            </Text>
                            <SpeedChart
                                data={speedHistory.down}
                                color={isProxyDead ? colors.textDark : colors.primary}
                            />
                        </View>
                        <View style={styles.statCard}>
                            <View style={styles.statHeader}>
                                <Text style={styles.statLabel}>{t('home.upload')}</Text>
                                <Text
                                    style={[
                                        styles.statSpeed,
                                        { color: isProxyDead ? colors.textDark : colors.primaryLight },
                                    ]}>
                                    {formatSpeed(speedHistory.up[19])}
                                </Text>
                            </View>
                            <Text
                                style={[
                                    styles.statValue,
                                    { color: isProxyDead ? colors.textDark : colors.primaryLight },
                                ]}>
                                {formatBytes(stats.upload)}
                            </Text>
                            <SpeedChart
                                data={speedHistory.up}
                                color={isProxyDead ? colors.textDark : colors.primaryLight}
                            />
                        </View>
                    </View>
                )
            )}
        </ScrollView>
    );
};

const styles = StyleSheet.create({
    scrollView: { flex: 1, backgroundColor: colors.bg },
    container: {
        alignItems: 'center',
        paddingHorizontal: 16,
        paddingVertical: 24,
        gap: 24,
    },
    statusSection: { alignItems: 'center', gap: 6 },
    statusTitle: { fontSize: 30, lineHeight: 36, fontWeight: '700' },
    statusDesc: { fontSize: 16, lineHeight: 24, color: colors.textMuted, textAlign: 'center' },

    powerSection: { alignItems: 'center', justifyContent: 'center', marginVertical: 16 },
    powerGlow: {
        position: 'absolute',
        width: 180,
        height: 180,
        borderRadius: 90,
        opacity: 0.5,
    },
    powerBtn: {
        width: 160,
        height: 160,
        borderRadius: 80,
        borderWidth: 3,
        alignItems: 'center',
        justifyContent: 'center',
        elevation: 12,
    },

    emptySection: { width: '100%', alignItems: 'center', gap: 16 },
    emptyText: { fontSize: 16, lineHeight: 24, color: colors.textSecondary, textAlign: 'center' },
    addCard: {
        width: '100%',
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.borderLight,
        borderStyle: 'dashed',
        borderRadius: 24,
        padding: 32,
        alignItems: 'center',
    },
    addIconWrap: {
        backgroundColor: colors.border,
        padding: 16,
        borderRadius: 999,
        marginBottom: 16,
    },
    addTitle: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.text, marginBottom: 4 },
    addDesc: { fontSize: 14, lineHeight: 20, color: colors.textMuted },

    proxyCard: {
        width: '100%',
        backgroundColor: colors.card,
        borderRadius: 24,
        borderWidth: 1,
        overflow: 'hidden',
    },
    proxyHeader: {
        padding: 18,
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
    },
    proxyHeaderLeft: { flexDirection: 'row', alignItems: 'center', gap: 16, flex: 1 },
    proxyIconWrap: {
        width: 48,
        height: 48,
        borderRadius: 16,
        alignItems: 'center',
        justifyContent: 'center',
    },
    proxyInfo: { flex: 1 },
    proxyLabel: { fontSize: 12, lineHeight: 16, color: colors.textSecondary, marginBottom: 2 },
    proxyName: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.text },
    proxyAddr: { fontSize: 14, lineHeight: 20, color: colors.textMuted, marginTop: 2, fontFamily: 'monospace' },
    proxyActions: { flexDirection: 'row', alignItems: 'center', gap: 4, marginLeft: 12 },
    editBtn: { padding: 8, borderRadius: 12 },

    proxyDropdown: { borderTopWidth: 1, borderTopColor: colors.border + '80', backgroundColor: colors.bg + 'cc' },
    proxyScrollList: { maxHeight: 280, padding: 8 },
    countryGroup: { marginBottom: 8 },
    countryHeader: { flexDirection: 'row', alignItems: 'center', gap: 8, paddingHorizontal: 12, paddingVertical: 4 },
    countryLabel: { fontSize: 12, lineHeight: 16, fontWeight: '700', color: colors.textMuted, letterSpacing: 1 },
    proxyListItem: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: 12,
        borderRadius: 16,
        backgroundColor: colors.card + '80',
        borderWidth: 1,
        borderColor: 'transparent',
        marginBottom: 4,
    },
    proxyListItemActive: {
        backgroundColor: colors.primary + '10',
        borderColor: colors.primary + '30',
    },
    proxyListItemLeft: { flexDirection: 'row', alignItems: 'center', gap: 12, flex: 1 },
    proxyListFlag: {
        width: 36,
        height: 36,
        borderRadius: 8,
        backgroundColor: colors.border + '80',
        borderWidth: 1,
        borderColor: colors.borderLight + '80',
        alignItems: 'center',
        justifyContent: 'center',
    },
    proxyListName: { fontSize: 16, lineHeight: 24, fontWeight: '700', color: colors.text },
    proxyListAddr: { fontSize: 14, lineHeight: 20, color: colors.textMuted, fontFamily: 'monospace', marginTop: 2 },
    proxyListRight: { flexDirection: 'row', alignItems: 'center', gap: 10, marginLeft: 8 },
    pingWrap: { flexDirection: 'row', alignItems: 'center', gap: 3 },
    pingText: { fontSize: 12, lineHeight: 16, color: colors.textMuted },
    dot: { width: 8, height: 8, borderRadius: 4 },

    openListBtn: { padding: 14, alignItems: 'center', borderTopWidth: 1, borderTopColor: colors.border + '50' },
    openListText: { fontSize: 14, lineHeight: 20, color: colors.textSecondary },

    errorActions: { flexDirection: 'row', gap: 12, width: '100%' },
    errorBtn: {
        flex: 1,
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.border,
        paddingVertical: 16,
        borderRadius: 24,
        alignItems: 'center',
    },
    errorBtnText: { fontSize: 14, lineHeight: 20, fontWeight: '700', color: colors.text },
    errorBtnAlt: {
        flex: 1,
        backgroundColor: colors.error + '10',
        borderWidth: 1,
        borderColor: colors.error + '30',
        paddingVertical: 16,
        borderRadius: 24,
        alignItems: 'center',
    },
    errorBtnAltText: { fontSize: 14, lineHeight: 20, fontWeight: '700', color: colors.error },

    statsRow: { flexDirection: 'row', gap: 16, width: '100%' },
    statCard: {
        flex: 1,
        backgroundColor: colors.card,
        borderRadius: 24,
        padding: 20,
        borderWidth: 1,
        borderColor: colors.border,
    },
    statHeader: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'flex-start' },
    statLabel: {
        fontSize: 12,
        lineHeight: 16,
        color: colors.textMuted,
        fontWeight: '700',
        textTransform: 'uppercase',
        letterSpacing: 2,
    },
    statSpeed: { fontSize: 12, lineHeight: 16, fontWeight: '700' },
    statValue: { fontSize: 24, lineHeight: 32, fontWeight: '700', marginTop: 2 },
});
