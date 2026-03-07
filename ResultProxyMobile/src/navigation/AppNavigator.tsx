import React from 'react';
import { StyleSheet, View, Text, Pressable } from 'react-native';
import { NavigationContainer } from '@react-navigation/native';
import { createBottomTabNavigator } from '@react-navigation/bottom-tabs';
import { createNativeStackNavigator } from '@react-navigation/native-stack';
import {
    Activity,
    ShoppingCart,
    Plus,
    List,
    Settings,
} from 'lucide-react-native';
import { HomeScreen } from '../screens/HomeScreen';
import { ProxyListScreen } from '../screens/ProxyListScreen';
import { AddProxyScreen } from '../screens/AddProxyScreen';
import { BuyProxyScreen } from '../screens/BuyProxyScreen';
import { SettingsScreen } from '../screens/SettingsScreen';
import { RulesScreen } from '../screens/RulesScreen';
import { LogsScreen } from '../screens/LogsScreen';
import { colors } from '../theme';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Header } from '../components/layout/Header';

const Tab = createBottomTabNavigator();
const SettingsStack = createNativeStackNavigator();

const SettingsStackScreen = () => (
    <SettingsStack.Navigator
        screenOptions={{
            headerShown: false,
            contentStyle: { backgroundColor: colors.bg },
            animation: 'slide_from_right',
        }}>
        <SettingsStack.Screen name="SettingsMain" component={SettingsScreen} />
        <SettingsStack.Screen name="Rules" component={RulesScreen} />
        <SettingsStack.Screen name="Logs" component={LogsScreen} />
    </SettingsStack.Navigator>
);

const TAB_ICONS: Record<string, any> = {
    Home: Activity,
    Buy: ShoppingCart,
    AddProxy: Plus,
    ProxyList: List,
    SettingsTab: Settings,
};

const CustomTabBar = ({ state, descriptors, navigation }: any) => {
    const insets = useSafeAreaInsets();

    const labels: Record<string, string> = {
        Home: 'Главная',
        Buy: 'Купить',
        AddProxy: 'Добавить',
        ProxyList: 'Прокси',
        SettingsTab: 'Настройки',
    };

    return (
        <View style={[tabStyles.container, { paddingBottom: Math.max(8, insets.bottom) }]}>
            {state.routes.map((route: any, index: number) => {
                const isFocused = state.index === index;
                const IconComponent = TAB_ICONS[route.name] || Activity;
                const label = labels[route.name] || route.name;

                const onPress = () => {
                    const event = navigation.emit({
                        type: 'tabPress',
                        target: route.key,
                        canPreventDefault: true,
                    });
                    if (!isFocused && !event.defaultPrevented) {
                        navigation.navigate(route.name, route.params);
                    }
                };

                return (
                    <Pressable
                        key={route.key}
                        onPress={onPress}
                        style={tabStyles.tab}
                        android_ripple={{ color: colors.border, borderless: true }}>
                        <IconComponent
                            size={24}
                            color={isFocused ? colors.primary : colors.textMuted}
                        />
                        <Text
                            style={[
                                tabStyles.label,
                                { color: isFocused ? colors.primary : colors.textMuted },
                            ]}>
                            {label}
                        </Text>
                    </Pressable>
                );
            })}
        </View>
    );
};

export const AppNavigator = () => (
    <NavigationContainer>
        <View style={{ flex: 1, backgroundColor: colors.bg }}>
            <Header />
            <Tab.Navigator
                tabBar={props => <CustomTabBar {...props} />}
                screenOptions={{
                    headerShown: false,
                }}>
                <Tab.Screen name="Home" component={HomeScreen} />
                <Tab.Screen name="Buy" component={BuyProxyScreen} />
                <Tab.Screen name="AddProxy" component={AddProxyScreen} />
                <Tab.Screen name="ProxyList" component={ProxyListScreen} />
                <Tab.Screen name="SettingsTab" component={SettingsStackScreen} />
            </Tab.Navigator>
        </View>
    </NavigationContainer>
);

const tabStyles = StyleSheet.create({
    container: {
        flexDirection: 'row',
        backgroundColor: colors.card,
        borderTopWidth: 1,
        borderTopColor: colors.border,
        paddingTop: 8,
        paddingHorizontal: 8,
        justifyContent: 'space-around',
    },
    tab: {
        alignItems: 'center',
        justifyContent: 'center',
        padding: 8,
        minWidth: 64,
        gap: 4,
    },
    label: {
        fontSize: 10,
        fontWeight: '500',
    },
});
