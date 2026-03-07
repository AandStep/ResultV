import React, { useCallback, useState } from 'react';
import {
    View,
    Text,
    TextInput,
    Pressable,
    ScrollView,
    Modal,
    Alert,
    StyleSheet,
} from 'react-native';
import {
    Shield,
    Power,
    Upload,
    Download,
    ArrowRight,
    FileText,
    Globe,
} from 'lucide-react-native';
import { useTranslation } from 'react-i18next';
import { SettingToggle } from '../components/ui/SettingToggle';
import { useConfigStore } from '../store/configStore';
import { useLogStore } from '../store/logStore';
import { useConnectionStore } from '../store/connectionStore';
import { colors } from '../theme';

export const SettingsScreen = ({ navigation }: any) => {
    const { t, i18n } = useTranslation();
    const settings = useConfigStore(s => s.settings);
    const updateSetting = useConfigStore(s => s.updateSetting);
    const proxies = useConfigStore(s => s.proxies);
    const routingRules = useConfigStore(s => s.routingRules);
    const setProxies = useConfigStore(s => s.setProxies);
    const setRoutingRules = useConfigStore(s => s.setRoutingRules);
    const setSettings = useConfigStore(s => s.setSettings);
    const addLog = useLogStore(s => s.addLog);
    const daemonStatus = useConnectionStore(s => s.daemonStatus);

    const [showPasswordModal, setShowPasswordModal] = useState(false);
    const [passwordModalMode, setPasswordModalMode] = useState<'export' | 'import'>('export');
    const [password, setPassword] = useState('');

    const handleExport = useCallback(() => {
        setPasswordModalMode('export');
        setShowPasswordModal(true);
        setPassword('');
    }, []);

    const handleImport = useCallback(() => {
        setPasswordModalMode('import');
        setShowPasswordModal(true);
        setPassword('');
    }, []);

    const handlePasswordSubmit = useCallback(async () => {
        if (password.length < 4) {
            Alert.alert(t('settings.error'), t('settings.passwordTooShort'));
            return;
        }

        if (passwordModalMode === 'export') {
            try {
                const data = { proxies, routingRules, settings };
                const jsonStr = JSON.stringify(data, null, 2);

                const { Share } = require('react-native');
                await Share.share({ message: jsonStr, title: 'ResultProxy Config' });
                addLog('Конфигурация экспортирована.', 'success');
            } catch {
                addLog('Ошибка экспорта.', 'error');
            }
        } else {
            Alert.alert(
                t('settings.export_import.import_btn'),
                t('settings.importInstructions'),
            );
        }

        setShowPasswordModal(false);
        setPassword('');
    }, [password, passwordModalMode, proxies, routingRules, settings, addLog, t]);

    return (
        <ScrollView style={styles.scrollView} contentContainerStyle={styles.container}>
            <View style={styles.headerSection}>
                <View style={styles.headerRow}>
                    <Text style={styles.title}>{t('settings.title')}</Text>
                </View>
                <Text style={styles.desc}>{t('settings.desc')}</Text>
            </View>

            <View style={styles.navSection}>
                <Pressable
                    onPress={() => {
                        const nextLang = i18n.language?.startsWith('ru') ? 'en' : 'ru';
                        i18n.changeLanguage(nextLang);
                    }}
                    style={styles.navItem}
                    android_ripple={{ color: colors.border }}>
                    <View style={styles.navLeft}>
                        <Globe size={20} color={colors.textSecondary} />
                        <Text style={styles.navText}>{t('settings.language_toggle')}</Text>
                    </View>
                    <Text style={{ color: colors.primaryLight, fontSize: 14, lineHeight: 20, fontWeight: '600' }}>
                        {t('settings.current_language')}
                    </Text>
                </Pressable>
                <Pressable
                    onPress={() => navigation.navigate('Rules')}
                    style={styles.navItem}
                    android_ripple={{ color: colors.border }}>
                    <View style={styles.navLeft}>
                        <Shield size={20} color={colors.textSecondary} />
                        <Text style={styles.navText}>{t('sidebar.rules')}</Text>
                    </View>
                    <ArrowRight size={18} color={colors.textMuted} />
                </Pressable>
                <Pressable
                    onPress={() => navigation.navigate('Logs')}
                    style={styles.navItem}
                    android_ripple={{ color: colors.border }}>
                    <View style={styles.navLeft}>
                        <FileText size={20} color={colors.textSecondary} />
                        <Text style={styles.navText}>{t('sidebar.logs')}</Text>
                    </View>
                    <ArrowRight size={18} color={colors.textMuted} />
                </Pressable>
            </View>

            <View style={styles.togglesSection}>
                <SettingToggle
                    title={t('settings.killswitch.title')}
                    description={t('settings.killswitch.desc')}
                    isOn={settings.killswitch}
                    onToggle={() => updateSetting('killswitch', !settings.killswitch)}
                />
            </View>

            <View style={styles.configCard}>
                <Text style={styles.sectionTitle}>{t('settings.export_import.title')}</Text>
                <View style={styles.configDescContainer}>
                    <Shield size={16} color={colors.primary} />
                    <Text style={styles.configDescText}>{t('settings.export_import.desc')}</Text>
                </View>
                <View style={styles.configButtons}>
                    <Pressable
                        onPress={handleExport}
                        style={styles.configBtn}
                        android_ripple={{ color: colors.border }}>
                        <Download size={20} color={colors.text} />
                        <Text style={styles.configBtnText}>{t('settings.export_import.export_btn')}</Text>
                    </Pressable>
                    <Pressable
                        onPress={handleImport}
                        style={styles.configBtn}
                        android_ripple={{ color: colors.border }}>
                        <Upload size={20} color={colors.text} />
                        <Text style={styles.configBtnText}>{t('settings.export_import.import_btn')}</Text>
                    </Pressable>
                </View>
            </View>

            <Modal
                visible={showPasswordModal}
                transparent
                animationType="fade"
                onRequestClose={() => setShowPasswordModal(false)}>
                <View style={styles.modalOverlay}>
                    <View style={styles.modal}>
                        <Text style={styles.modalTitle}>
                            {passwordModalMode === 'export'
                                ? t('settings.modal.title_export')
                                : t('settings.modal.title_import')}
                        </Text>
                        <TextInput
                            placeholder={t('settings.modal.placeholder')}
                            placeholderTextColor={colors.textMuted}
                            style={styles.modalInput}
                            value={password}
                            onChangeText={setPassword}
                            secureTextEntry
                            autoFocus
                        />
                        <View style={styles.modalActions}>
                            <Pressable
                                onPress={() => setShowPasswordModal(false)}
                                style={styles.modalCancel}
                                android_ripple={{ color: colors.border }}>
                                <Text style={styles.modalCancelText}>{t('add.cancel')}</Text>
                            </Pressable>
                            <Pressable
                                onPress={handlePasswordSubmit}
                                style={styles.modalConfirm}
                                android_ripple={{ color: colors.primaryDark }}>
                                <Text style={styles.modalConfirmText}>
                                    {passwordModalMode === 'export'
                                        ? t('settings.export_import.export_btn')
                                        : t('settings.export_import.import_btn')}
                                </Text>
                            </Pressable>
                        </View>
                    </View>
                </View>
            </Modal>
        </ScrollView>
    );
};

const styles = StyleSheet.create({
    scrollView: { flex: 1, backgroundColor: colors.bg },
    container: { padding: 16, gap: 20, paddingBottom: 40 },
    headerSection: {},
    headerRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
    title: { fontSize: 30, lineHeight: 36, fontWeight: '700', color: colors.text },
    desc: { fontSize: 16, lineHeight: 24, color: colors.textSecondary, marginTop: 6 },

    togglesSection: { gap: 14 },

    navSection: {
        backgroundColor: colors.card,
        borderRadius: 20,
        borderWidth: 1,
        borderColor: colors.border,
        overflow: 'hidden',
    },
    navItem: {
        flexDirection: 'row',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: 18,
        borderBottomWidth: 1,
        borderBottomColor: colors.border,
    },
    navLeft: { flexDirection: 'row', alignItems: 'center', gap: 14 },
    navText: { fontSize: 16, lineHeight: 24, fontWeight: '600', color: colors.text },

    configCard: {
        backgroundColor: colors.card,
        padding: 24,
        borderRadius: 24,
        borderWidth: 1,
        borderColor: colors.border,
        gap: 16,
    },
    sectionTitle: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.text },
    configDescContainer: { flexDirection: 'row', gap: 8, alignItems: 'flex-start' },
    configDescText: { fontSize: 14, lineHeight: 20, color: colors.textSecondary, flex: 1 },
    configButtons: { flexDirection: 'row', gap: 12 },
    configBtn: {
        flex: 1,
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 10,
        backgroundColor: colors.border,
        paddingVertical: 14,
        borderRadius: 16,
    },
    configBtnText: { fontSize: 16, lineHeight: 24, fontWeight: '600', color: colors.text },

    modalOverlay: {
        flex: 1,
        backgroundColor: 'rgba(0,0,0,0.5)',
        justifyContent: 'center',
        alignItems: 'center',
        padding: 16,
    },
    modal: {
        width: '100%',
        maxWidth: 380,
        backgroundColor: colors.bg,
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 16,
        padding: 24,
    },
    modalTitle: { fontSize: 20, lineHeight: 28, fontWeight: '700', color: colors.text, marginBottom: 16 },
    modalInput: {
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.border,
        borderRadius: 12,
        paddingHorizontal: 16,
        paddingVertical: 12,
        color: colors.text,
        fontSize: 16,
        lineHeight: 24,
        marginBottom: 20,
    },
    modalActions: { flexDirection: 'row', gap: 12 },
    modalCancel: {
        flex: 1,
        paddingVertical: 14,
        borderRadius: 12,
        backgroundColor: colors.card,
        borderWidth: 1,
        borderColor: colors.border,
        alignItems: 'center',
    },
    modalCancelText: { fontSize: 14, lineHeight: 20, fontWeight: '700', color: colors.textSecondary },
    modalConfirm: {
        flex: 1,
        paddingVertical: 14,
        borderRadius: 12,
        backgroundColor: colors.primary,
        alignItems: 'center',
    },
    modalConfirmText: { fontSize: 14, lineHeight: 20, fontWeight: '700', color: colors.text },
});
