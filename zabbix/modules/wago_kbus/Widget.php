<?php
namespace Modules\WagoKbus;

class Widget extends \Zabbix\Core\CWidget {
    public function getDefaultName(): string {
        return 'WAGO K-bus Visualizer';
    }
}
