<?php

// Load Laravel's autoloader
require_once __DIR__ . '/vendor/autoload.php';

// Bootstrap the Laravel application
$app = require_once __DIR__ . '/bootstrap/app.php';

// Make the application instance available in PsySH
return [
    'startup' => function ($psy) use ($app) {
        // You can add custom helpers or bindings here if needed
    },
    // Set the history file location to the persistent volume
    'historyFile' => getenv('PSYSH_HOME') . '/psysh_history',
];
