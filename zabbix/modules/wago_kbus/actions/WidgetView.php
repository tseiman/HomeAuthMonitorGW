<?php
namespace Modules\WagoKbus\Actions;

class WidgetView extends \CControllerDashboardWidgetView {

    protected function doAction(): void {
        $data = [
            'name'   => $this->getInput('name', $this->widget->getDefaultName()),
            'layout' => null,
            'clock'  => 0,
            'error'  => null,
            'user'   => ['debug_mode' => $this->getDebugMode()],
        ];

        try {
            $hostids = $this->fields_values['hostid'] ?? [];
            if (count($hostids) !== 1) {
                throw new \RuntimeException(_('Select exactly one WAGO device host.'));
            }

            $items = \API::Item()->get([
                'hostids' => $hostids,
                'output'  => ['itemid', 'value_type'],
                'filter'  => ['key_' => 'automation.gateway.wago.kbus_layout'],
                'limit'   => 1,
            ]);
            if (!$items) {
                throw new \RuntimeException(_(
                    'Item automation.gateway.wago.kbus_layout not found on the selected host. ' .
                    'Import or update the HomeAuthMonitorGW template (version that includes this item).'
                ));
            }

            $history = \API::History()->get([
                'itemids'   => [$items[0]['itemid']],
                'output'    => ['clock', 'value'],
                'sortfield' => 'clock',
                'sortorder' => 'DESC',
                'limit'     => 1,
                'history'   => ITEM_VALUE_TYPE_TEXT,
            ]);
            if (!$history) {
                throw new \RuntimeException(_(
                    'No data yet for K-bus layout item. Wait for the first collection cycle.'
                ));
            }

            $layout = json_decode($history[0]['value'], true);
            if (!is_array($layout) || !array_key_exists('modules', $layout) || !is_array($layout['modules'])) {
                throw new \RuntimeException(_('K-bus layout item returned unexpected JSON structure.'));
            }

            $data['layout'] = $layout;
            $data['clock']  = (int) $history[0]['clock'];
        } catch (\Throwable $e) {
            $data['error'] = $e->getMessage();
        }

        $this->setResponse(new \CControllerResponseData($data));
    }
}
