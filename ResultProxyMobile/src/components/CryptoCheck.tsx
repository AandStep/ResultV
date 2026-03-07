import { useEffect } from 'react';

const CryptoCheck = () => {
    useEffect(() => {
        console.log('--- Crypto Check ---');
        console.log('global.crypto:', !!global.crypto);
        if (global.crypto) {
            console.log('global.crypto.subtle:', !!global.crypto.subtle);
            console.log('global.crypto.getRandomValues:', !!global.crypto.getRandomValues);
        }
    }, []);
    return null;
};

export default CryptoCheck;
