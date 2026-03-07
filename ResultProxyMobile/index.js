import 'react-native-get-random-values';
import { install } from 'react-native-quick-crypto';
import { Buffer } from '@craftzdog/react-native-buffer';
import { AppRegistry } from 'react-native';
import App from './App';
import { name as appName } from './app.json';

global.Buffer = Buffer;
install();

AppRegistry.registerComponent(appName, () => App);
