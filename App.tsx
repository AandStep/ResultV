import React, { useEffect, useState } from 'react';
import { StatusBar, View, ActivityIndicator, StyleSheet } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { AppNavigator } from './src/navigation/AppNavigator';
import { useConfigStore } from './src/store/configStore';
import { useConnectionStore } from './src/store/connectionStore';
import { useLogStore } from './src/store/logStore';
import { UpdateModal } from './src/components/ui/UpdateModal';
import { compareVersions } from './src/utils/versionCheck';
import { apiFetch } from './src/services/api';
import { colors } from './src/theme';
import './src/lib/i18n';

const UPDATE_URL =
  'https://raw.githubusercontent.com/AandStep/ResultProxy/dev/update.json';

const App = () => {
  const loadConfig = useConfigStore(s => s.loadConfig);
  const isConfigLoaded = useConfigStore(s => s.isConfigLoaded);
  const proxies = useConfigStore(s => s.proxies);
  const addLog = useLogStore(s => s.addLog);
  const startStatusPolling = useConnectionStore(s => s.startStatusPolling);
  const startPingPolling = useConnectionStore(s => s.startPingPolling);
  const startLogPolling = useLogStore(s => s.startPolling);

  const [updateVisible, setUpdateVisible] = useState(false);
  const [currentVersion, setCurrentVersion] = useState('');
  const [latestData, setLatestData] = useState<any>(null);

  useEffect(() => {
    loadConfig(addLog);
  }, [loadConfig, addLog]);

  useEffect(() => {
    if (!isConfigLoaded) return;
    const stopStatus = startStatusPolling(proxies, addLog);
    const stopPing = startPingPolling(proxies);
    const stopLogs = startLogPolling();
    return () => {
      stopStatus();
      stopPing();
      stopLogs();
    };
  }, [isConfigLoaded, proxies, addLog, startStatusPolling, startPingPolling, startLogPolling]);

  useEffect(() => {
    const checkUpdate = async () => {
      try {
        const localRes = await apiFetch('/api/version');
        const localData = await localRes.json();
        setCurrentVersion(localData.version);

        const cacheBuster = `?_t=${Date.now()}`;
        const remoteRes = await fetch(`${UPDATE_URL}${cacheBuster}`);
        const remoteData = await remoteRes.json();
        setLatestData(remoteData);

        if (localData.version && remoteData.version) {
          const isNewer =
            compareVersions(localData.version, remoteData.version) === -1;
          setUpdateVisible(isNewer);
        }
      } catch { }
    };
    checkUpdate();
  }, []);

  if (!isConfigLoaded) {
    return (
      <View style={styles.loader}>
        <StatusBar barStyle="light-content" backgroundColor={colors.bg} />
        <ActivityIndicator size="large" color={colors.primary} />
      </View>
    );
  }

  return (
    <GestureHandlerRootView style={styles.root}>
      <SafeAreaProvider>
        <StatusBar barStyle="light-content" backgroundColor={colors.bg} />
        <AppNavigator />
        <UpdateModal
          visible={updateVisible}
          currentVersion={currentVersion}
          latestVersion={latestData?.version}
          downloadUrl={latestData?.downloadUrl}
          onClose={() => setUpdateVisible(false)}
        />
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
};

const styles = StyleSheet.create({
  root: { flex: 1 },
  loader: {
    flex: 1,
    backgroundColor: colors.bg,
    alignItems: 'center',
    justifyContent: 'center',
  },
});

export default App;
