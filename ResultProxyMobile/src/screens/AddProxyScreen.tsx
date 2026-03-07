import React, { useCallback, useEffect, useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet } from 'react-native';
import { Lock } from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import { useConfigStore, ProxyItem } from '../store/configStore';
import { useConnectionStore } from '../store/connectionStore';
import { useLogStore } from '../store/logStore';
import { colors } from '../theme';

export const AddProxyScreen = ({ navigation }: any) => {
    const { t } = useTranslation();
    const editingProxy = useConfigStore(s => s.editingProxy);
    const setEditingProxy = useConfigStore(s => s.setEditingProxy);
    const handleSaveProxy = useConfigStore(s => s.handleSaveProxy);
    const routingRules = useConfigStore(s => s.routingRules);
    const settings = useConfigStore(s => s.settings);

    const activeProxy = useConnectionStore(s => s.activeProxy);
    const failedProxy = useConnectionStore(s => s.failedProxy);
    const setFailedProxy = useConnectionStore(s => s.setFailedProxy);
    const setActiveProxy = useConnectionStore(s => s.setActiveProxy);
    const isConnected = useConnectionStore(s => s.isConnected);
    const selectAndConnect = useConnectionStore(s => s.selectAndConnect);
    const addLog = useLogStore(s => s.addLog);

    const [formData, setFormData] = useState({
        name: '',
        ip: '',
        port: '',
        type: 'SOCKS5',
        username: '',
        password: '',
        country: '🌐',
    });

    useEffect(() => {
        if (editingProxy) {
            setFormData({
                name: editingProxy.name || '',
                ip: editingProxy.ip || '',
                port: editingProxy.port || '',
                type: editingProxy.type || 'SOCKS5',
                username: editingProxy.username || '',
                password: editingProxy.password || '',
                country: editingProxy.country || '🌐',
            });
        } else {
            setFormData({ name: '', ip: '', port: '', type: 'SOCKS5', username: '', password: '', country: '🌐' });
        }
    }, [editingProxy]);

    const handleSubmit = useCallback(() => {
        if (!formData.ip || !formData.port) return;

        const proxyData = {
            ...formData,
            ...(editingProxy?.id ? { id: editingProxy.id } : {}),
            name: formData.name || t('add.newServer'),
        };

        handleSaveProxy(
            proxyData,
            activeProxy,
            failedProxy,
            setFailedProxy,
            setActiveProxy,
            isConnected,
            (proxy: ProxyItem, force?: boolean) =>
                selectAndConnect(proxy, routingRules, settings.killswitch, addLog, force),
            addLog,
        );
        navigation.goBack();
    }, [
        formData, editingProxy, handleSaveProxy, activeProxy, failedProxy,
        setFailedProxy, setActiveProxy, isConnected, selectAndConnect,
        routingRules, settings.killswitch, addLog, navigation, t,
    ]);

    const updateField = useCallback((key: string, value: string) => {
        setFormData(prev => ({ ...prev, [key]: value }));
    }, []);

    return (
        <ScrollView style={styles.scrollView} contentContainerStyle={styles.container}>
            <View style={styles.headerSection}>
                <Text style={styles.title}>
                    {editingProxy ? t('add.titleEdit') : t('add.titleAdd')}
                </Text>
                <Text style={styles.desc}>
                    {editingProxy ? t('add.descEdit') : t('add.descAdd')}
                </Text>
            </View>

            <View style={styles.form}>
                <View>
                    <Text style={styles.label}>{t('add.profileName')}</Text>
                    <TextInput
                        placeholder={t('add.profilePlaceholder')}
                        placeholderTextColor={colors.textMuted}
                        style={styles.input}
                        value={formData.name}
                        onChangeText={v => updateField('name', v)}
                    />
                </View>

                <View style={styles.row}>
                    <View style={styles.flex2}>
                        <Text style={styles.label}>{t('add.ip')}</Text>
                        <TextInput
                            placeholder="192.168.1.1"
                            placeholderTextColor={colors.textMuted}
                            style={styles.input}
                            value={formData.ip}
                            onChangeText={v => updateField('ip', v)}
                            autoCapitalize="none"
                        />
                    </View>
                    <View style={styles.flex1}>
                        <Text style={styles.label}>{t('add.port')}</Text>
                        <TextInput
                            placeholder="8000"
                            placeholderTextColor={colors.textMuted}
                            style={styles.input}
                            value={formData.port}
                            onChangeText={v => updateField('port', v)}
                            keyboardType="numeric"
                        />
                    </View>
                </View>

                <View>
                    <Text style={styles.label}>{t('add.protocol')}</Text>
                    <View style={styles.protocolRow}>
                        {['HTTP', 'HTTPS', 'SOCKS5'].map(type => (
                            <Pressable
                                key={type}
                                onPress={() => updateField('type', type)}
                                style={[
                                    styles.protocolBtn,
                                    formData.type === type && styles.protocolBtnActive,
                                ]}>
                                <Text
                                    style={[
                                        styles.protocolText,
                                        formData.type === type && styles.protocolTextActive,
                                    ]}>
                                    {type}
                                </Text>
                            </Pressable>
                        ))}
                    </View>
                </View>

                <View style={styles.authSection}>
                    <View style={styles.authHeader}>
                        <Lock size={16} color={colors.textSecondary} />
                        <Text style={styles.authLabel}>{t('add.auth')}</Text>
                    </View>
                    <View style={styles.authFields}>
                        <TextInput
                            placeholder={t('add.loginPlaceholder')}
                            placeholderTextColor={colors.textMuted}
                            style={[styles.input, styles.flex1]}
                            value={formData.username}
                            onChangeText={v => updateField('username', v)}
                            autoCapitalize="none"
                        />
                        <TextInput
                            placeholder={t('add.passPlaceholder')}
                            placeholderTextColor={colors.textMuted}
                            style={[styles.input, styles.flex1]}
                            value={formData.password}
                            onChangeText={v => updateField('password', v)}
                            secureTextEntry
                        />
                    </View>
                </View>

                <View style={styles.submitRow}>
                    {editingProxy && (
                        <Pressable
                            onPress={() => {
                                setEditingProxy(null);
                                navigation.goBack();
                            }}
                            style={styles.cancelBtn}
                            android_ripple={{ color: colors.border }}>
                            <Text style={styles.cancelText}>{t('add.cancel')}</Text>
                        </Pressable>
                    )}
                    <Pressable
                        onPress={handleSubmit}
                        style={[styles.saveBtn, !editingProxy && { flex: 1 }]}
                        android_ripple={{ color: colors.primaryDark }}>
                        <Text style={styles.saveText}>
                            {editingProxy ? t('add.saveChanges') : t('add.saveProxy')}
                        </Text>
                    </Pressable>
                </View>
            </View>
        </ScrollView>
    );
};

const styles = StyleSheet.create({
    scrollView: { flex: 1, backgroundColor: colors.bg },
    container: { padding: 16, gap: 16 },
    headerSection: {},
    title: { fontSize: 30, lineHeight: 36, fontWeight: '700', color: colors.text },
    desc: { fontSize: 16, lineHeight: 24, color: colors.textSecondary, marginTop: 6 },

    form: {
        backgroundColor: colors.card,
        padding: 20,
        borderRadius: 24,
        borderWidth: 1,
        borderColor: colors.border,
        gap: 18,
    },
    label: { fontSize: 14, lineHeight: 20, fontWeight: '500', color: colors.textSecondary, marginBottom: 8 },
    input: {
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
    row: { flexDirection: 'row', gap: 14 },
    flex1: { flex: 1 },
    flex2: { flex: 2 },

    protocolRow: { flexDirection: 'row', gap: 10 },
    protocolBtn: {
        flex: 1,
        paddingVertical: 12,
        borderRadius: 12,
        backgroundColor: colors.bg,
        borderWidth: 1,
        borderColor: colors.border,
        alignItems: 'center',
    },
    protocolBtnActive: { backgroundColor: colors.primary, borderColor: colors.primary },
    protocolText: { fontSize: 14, lineHeight: 20, fontWeight: '700', color: colors.textSecondary },
    protocolTextActive: { color: colors.text },

    authSection: {
        paddingTop: 16,
        borderTopWidth: 1,
        borderTopColor: colors.border,
    },
    authHeader: { flexDirection: 'row', alignItems: 'center', gap: 8, marginBottom: 12 },
    authLabel: { fontSize: 14, lineHeight: 20, fontWeight: '500', color: colors.textSecondary },
    authFields: { gap: 12 },

    submitRow: { flexDirection: 'row', gap: 12, paddingTop: 8 },
    cancelBtn: {
        flex: 1,
        backgroundColor: colors.border,
        paddingVertical: 14,
        borderRadius: 12,
        alignItems: 'center',
    },
    cancelText: { fontSize: 16, lineHeight: 24, fontWeight: '700', color: colors.text },
    saveBtn: {
        flex: 2,
        backgroundColor: colors.primary,
        paddingVertical: 14,
        borderRadius: 12,
        alignItems: 'center',
        elevation: 4,
    },
    saveText: { fontSize: 16, lineHeight: 24, fontWeight: '700', color: colors.text },
});
