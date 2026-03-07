import React, { useCallback, useState } from 'react';
import {
    View,
    Text,
    TextInput,
    Pressable,
    ScrollView,
    StyleSheet,
} from 'react-native';
import { Globe, Radio, Trash2, Plus, Box } from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import DocumentPicker from 'react-native-document-picker';
import { useConfigStore } from '../store/configStore';
import { colors } from '../theme';
import { AppPickerModal } from '../components/ui/AppPickerModal';

export const RulesScreen = () => {
    const { t } = useTranslation();
    const routingRules = useConfigStore(s => s.routingRules);
    const setRoutingRules = useConfigStore(s => s.setRoutingRules);
    const [newDomain, setNewDomain] = useState('');

    const handleModeChange = useCallback(
        (mode: 'global' | 'smart') => {
            setRoutingRules({ ...routingRules, mode });
        },
        [routingRules, setRoutingRules],
    );

    const handleAddDomain = useCallback(() => {
        const domain = newDomain.trim().toLowerCase();
        if (!domain || routingRules.whitelist.includes(domain)) return;
        setRoutingRules({
            ...routingRules,
            whitelist: [...routingRules.whitelist, domain],
        });
        setNewDomain('');
    }, [newDomain, routingRules, setRoutingRules]);

    const handleRemoveDomain = useCallback(
        (domain: string) => {
            setRoutingRules({
                ...routingRules,
                whitelist: routingRules.whitelist.filter(d => d !== domain),
            });
        },
        [routingRules, setRoutingRules],
    );

    const handleRemoveApp = useCallback(
        (app: string) => {
            setRoutingRules({
                ...routingRules,
                appWhitelist: routingRules.appWhitelist.filter(a => a !== app),
            });
        },
        [routingRules, setRoutingRules],
    );

    const popularTlds = ["*.ru", "*.рф", "*.su", "*.by", "*.kz"];

    const [pickerVisible, setPickerVisible] = useState(false);

    const handleAppSelect = useCallback((packageName: string) => {
        if (!packageName || routingRules.appWhitelist.includes(packageName)) return;
        setRoutingRules({
            ...routingRules,
            appWhitelist: [...routingRules.appWhitelist, packageName],
        });
    }, [routingRules, setRoutingRules]);

    const handlePickApp = useCallback(() => {
        setPickerVisible(true);
    }, []);

    return (
        <ScrollView style={styles.scrollView} contentContainerStyle={styles.container}>
            <View style={styles.headerSection}>
                <Text style={styles.title}>{t('rules.title')}</Text>
                <Text style={styles.desc}>{t('rules.desc')}</Text>
            </View>

            <View style={styles.modeSection}>
                <Pressable
                    onPress={() => handleModeChange('global')}
                    style={[
                        styles.modeCard,
                        routingRules.mode === 'global' && styles.modeCardActive,
                    ]}
                    android_ripple={{ color: colors.border }}>
                    <Globe
                        size={32}
                        color={
                            routingRules.mode === 'global'
                                ? colors.primaryLight
                                : colors.textMuted
                        }
                    />
                    <View style={styles.modeInfo}>
                        <Text
                            style={[
                                styles.modeName,
                                routingRules.mode === 'global' && styles.modeNameActive,
                            ]}>
                            {t('rules.modes.global')}
                        </Text>
                        <Text style={styles.modeDesc}>{t('rules.modes.global_desc')}</Text>
                    </View>
                    <View
                        style={[
                            styles.radio,
                            routingRules.mode === 'global' && styles.radioActive,
                        ]}>
                        {routingRules.mode === 'global' && <View style={styles.radioDot} />}
                    </View>
                </Pressable>

                <Pressable
                    onPress={() => handleModeChange('smart')}
                    style={[
                        styles.modeCard,
                        routingRules.mode === 'smart' && styles.modeCardActive,
                    ]}
                    android_ripple={{ color: colors.border }}>
                    <Radio
                        size={32}
                        color={
                            routingRules.mode === 'smart'
                                ? colors.primaryLight
                                : colors.textMuted
                        }
                    />
                    <View style={styles.modeInfo}>
                        <Text
                            style={[
                                styles.modeName,
                                routingRules.mode === 'smart' && styles.modeNameActive,
                            ]}>
                            {t('rules.modes.smart')}
                        </Text>
                        <Text style={styles.modeDesc}>{t('rules.modes.smart_desc')}</Text>
                    </View>
                    <View
                        style={[
                            styles.radio,
                            routingRules.mode === 'smart' && styles.radioActive,
                        ]}>
                        {routingRules.mode === 'smart' && <View style={styles.radioDot} />}
                    </View>
                </Pressable>
            </View>

            <View style={styles.whitelistSection}>
                <Text style={styles.sectionTitle}>
                    {t('rules.domains.title')}
                </Text>
                <Text style={styles.sectionDesc}>
                    {t('rules.domains.desc')}
                </Text>

                <View style={styles.addRow}>
                    <TextInput
                        style={styles.domainInput}
                        placeholder={t('rules.domains.placeholder')}
                        placeholderTextColor={colors.textMuted}
                        value={newDomain}
                        onChangeText={setNewDomain}
                        autoCapitalize="none"
                        onSubmitEditing={handleAddDomain}
                    />
                    <Pressable
                        onPress={handleAddDomain}
                        style={styles.addDomainBtn}
                        android_ripple={{ color: colors.primaryDark }}>
                        <Text style={styles.addBtnText}>{t('rules.domains.add_btn')}</Text>
                    </Pressable>
                </View>

                <View style={styles.chipContainer}>
                    {routingRules.whitelist.map(domain => (
                        <View key={domain} style={styles.chip}>
                            <Text style={styles.chipText}>{domain}</Text>
                            <Pressable
                                onPress={() => handleRemoveDomain(domain)}
                                hitSlop={6}
                                style={styles.trashBtn}>
                                <Trash2 size={16} color={colors.textMuted} />
                            </Pressable>
                        </View>
                    ))}
                </View>

                <View style={styles.fastAddSection}>
                    <Text style={styles.fastAddTitle}>{t('rules.domains.fast_add')}</Text>
                    <View style={styles.fastAddRows}>
                        {popularTlds.map(tld => {
                            const isAdded = routingRules.whitelist.includes(tld);
                            return (
                                <Pressable
                                    key={tld}
                                    disabled={isAdded}
                                    onPress={() => {
                                        if (!isAdded) {
                                            setRoutingRules({
                                                ...routingRules,
                                                whitelist: [...routingRules.whitelist, tld],
                                            });
                                        }
                                    }}
                                    style={[styles.fastAddBtn, isAdded && styles.fastAddBtnActive]}>
                                    {!isAdded && <Plus size={14} color={colors.textSecondary} style={{ marginRight: 6 }} />}
                                    <Text style={[styles.fastAddBtnText, isAdded && styles.fastAddBtnTextActive]}>{tld}</Text>
                                </Pressable>
                            );
                        })}
                    </View>
                </View>
            </View>

            <View style={styles.whitelistSection}>
                <Text style={styles.sectionTitle}>
                    {t('rules.apps.title')}
                </Text>
                <Text style={styles.sectionDesc}>
                    {t('rules.apps.desc1')}
                </Text>

                <View style={styles.appAddSection}>
                    <Pressable
                        onPress={handlePickApp}
                        style={styles.appPickBtn}
                        android_ripple={{ color: colors.primaryDark }}>
                        <Text style={styles.addBtnText}>{t('rules.apps.select')}</Text>
                    </Pressable>
                </View>

                <View style={styles.chipContainer}>
                    {routingRules.appWhitelist.map(app => (
                        <View key={app} style={styles.chip}>
                            <Box size={14} color={colors.textSecondary} style={{ marginRight: 2 }} />
                            <Text style={styles.chipText}>{app}</Text>
                            <Pressable
                                onPress={() => handleRemoveApp(app)}
                                hitSlop={6}
                                style={styles.trashBtn}>
                                <Trash2 size={16} color={colors.textMuted} />
                            </Pressable>
                        </View>
                    ))}
                </View>
            </View>

            <AppPickerModal
                visible={pickerVisible}
                onClose={() => setPickerVisible(false)}
                onSelect={handleAppSelect}
            />
        </ScrollView>
    );
};

const styles = StyleSheet.create({
    scrollView: { flex: 1, backgroundColor: colors.bg },
    container: { padding: 16, gap: 20 },
    headerSection: {},
    title: { fontSize: 30, lineHeight: 36, fontWeight: '700', color: colors.text },
    desc: { fontSize: 16, lineHeight: 24, color: colors.textSecondary, marginTop: 6 },

    modeSection: { gap: 14 },
    modeCard: {
        flexDirection: 'row',
        alignItems: 'center',
        padding: 20,
        backgroundColor: colors.card,
        borderRadius: 24,
        borderWidth: 1,
        borderColor: colors.border,
        gap: 16,
    },
    modeCardActive: { borderColor: colors.primaryLight + '60', backgroundColor: colors.primary + '08' },
    modeInfo: { flex: 1 },
    modeName: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.text },
    modeNameActive: { color: colors.primaryLight },
    modeDesc: { fontSize: 16, lineHeight: 24, color: colors.textMuted, marginTop: 4 },
    radio: {
        width: 22,
        height: 22,
        borderRadius: 11,
        borderWidth: 2,
        borderColor: colors.borderLight,
        alignItems: 'center',
        justifyContent: 'center',
    },
    radioActive: { borderColor: colors.primaryLight },
    radioDot: { width: 12, height: 12, borderRadius: 6, backgroundColor: colors.primaryLight },

    whitelistSection: {
        backgroundColor: colors.card,
        padding: 20,
        borderRadius: 24,
        borderWidth: 1,
        borderColor: colors.border,
    },
    sectionTitle: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.text },
    sectionDesc: { fontSize: 16, lineHeight: 24, color: colors.textMuted, marginTop: 4, marginBottom: 16 },
    addRow: { flexDirection: 'row', gap: 10, marginBottom: 14 },
    domainInput: {
        flex: 1,
        backgroundColor: colors.bg,
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 12,
        paddingHorizontal: 16,
        paddingVertical: 12,
        color: colors.text,
        fontSize: 16,
        lineHeight: 24,
    },
    addDomainBtn: {
        backgroundColor: colors.primary,
        paddingHorizontal: 20,
        paddingVertical: 14,
        borderRadius: 12,
        alignItems: 'center',
        justifyContent: 'center',
    },
    appAddSection: { gap: 10, marginBottom: 14 },
    appAddButtons: { flexDirection: 'row', gap: 10 },
    appAddBtn: {
        flex: 1,
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.border,
        paddingHorizontal: 20,
        paddingVertical: 14,
        borderRadius: 12,
        alignItems: 'center',
        justifyContent: 'center',
    },
    appPickBtn: {
        flex: 1,
        backgroundColor: colors.primary,
        paddingHorizontal: 20,
        paddingVertical: 14,
        borderRadius: 12,
        alignItems: 'center',
        justifyContent: 'center',
    },
    addBtnText: {
        color: colors.text,
        fontSize: 16,
        lineHeight: 24,
        fontWeight: 'bold',
    },
    trashBtn: {
        padding: 4,
    },
    chipContainer: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
    chip: {
        flexDirection: 'row',
        alignItems: 'center',
        backgroundColor: colors.border,
        borderWidth: 1,
        borderColor: colors.borderLight,
        borderRadius: 10,
        paddingHorizontal: 12,
        paddingVertical: 6,
        gap: 6,
    },
    chipText: { fontSize: 14, lineHeight: 20, color: colors.text },

    fastAddSection: { marginTop: 24, paddingTop: 20, borderTopWidth: 1, borderTopColor: colors.border },
    fastAddTitle: { fontSize: 14, lineHeight: 20, color: colors.textSecondary, marginBottom: 12, fontWeight: '500' },
    fastAddRows: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
    fastAddBtn: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 14,
        paddingVertical: 8,
        backgroundColor: colors.bg,
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 12,
    },
    fastAddBtnActive: {
        backgroundColor: colors.primary + '15',
        borderColor: colors.primary + '30',
    },
    fastAddBtnText: {
        fontSize: 14,
        lineHeight: 20,
        fontWeight: '500',
        color: colors.textSecondary,
    },
    fastAddBtnTextActive: {
        color: colors.primaryLight,
    },
});
