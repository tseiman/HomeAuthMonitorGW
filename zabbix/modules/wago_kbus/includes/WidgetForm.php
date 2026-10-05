<?php
namespace Modules\WagoKbus\Includes;

use Zabbix\Widgets\{CWidgetForm, CWidgetField};
use Zabbix\Widgets\Fields\CWidgetFieldMultiSelectHost;

class WidgetForm extends CWidgetForm {
    public function addFields(): self {
        return $this->addField(
            (new CWidgetFieldMultiSelectHost('hostid', _('WAGO device host')))
                ->setMultiple(false)
                ->setFlags(CWidgetField::FLAG_NOT_EMPTY | CWidgetField::FLAG_LABEL_ASTERISK)
        );
    }
}
