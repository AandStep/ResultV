<?php
// Для работы этого скрипта потребуется установить библиотеку MaxMind:
// Выполните команду в папке countryAPI на сервере: composer require maxmind-db/reader

require 'vendor/autoload.php';
use MaxMind\Db\Reader;

header('Content-Type: application/json');

$ip = $_GET['ip'] ?? '';
if (!$ip) {
    echo json_encode(['country' => '🌐']);
    exit;
}

try {
    // ВАЖНО: нужно переключиться на базу City (она весит ~30-40мб и там есть все нужные координаты и таймзоны)
    $reader = new Reader(__DIR__ . '/GeoLite2-City.mmdb');
    $record = $reader->get($ip);
    
    // Пытаемся достать физическую страну (обычно точнее), либо зарегистрированную
    $country = '';
    if (isset($record['country']['iso_code'])) {
        $country = strtolower($record['country']['iso_code']);
    } elseif (isset($record['registered_country']['iso_code'])) {
        $country = strtolower($record['registered_country']['iso_code']);
    }

    $response = [
        'country' => strtoupper($country ?: '🌐'),
        'country_lower' => strtolower($country ?: '🌐'),
        'region' => isset($record['subdivisions'][0]['iso_code']) ? $record['subdivisions'][0]['iso_code'] : '',
        'eu' => (isset($record['country']['is_in_european_union']) && $record['country']['is_in_european_union']) ? '1' : '0',
        'timezone' => isset($record['location']['time_zone']) ? $record['location']['time_zone'] : '',
        'city' => isset($record['city']['names']['en']) ? $record['city']['names']['en'] : '',
        'll' => [
            isset($record['location']['latitude']) ? $record['location']['latitude'] : 0,
            isset($record['location']['longitude']) ? $record['location']['longitude'] : 0
        ],
        'metro' => isset($record['location']['metro_code']) ? $record['location']['metro_code'] : 0,
        'area' => isset($record['location']['accuracy_radius']) ? $record['location']['accuracy_radius'] : 0
    ];
    
    echo json_encode($response);
    $reader->close();
} catch (Exception $e) {
    echo json_encode(['country' => '🌐', 'error' => $e->getMessage()]);
}
?>
