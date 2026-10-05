<?php
$form = new CWidgetFormView($data);
$form->addField(new CWidgetFieldMultiSelectHostView($data['fields']['hostid']));
$form->show();
