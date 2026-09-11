SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
-- Preserve customized units, options and other metadata. Enable amount entry
-- only for medication activities that do not already track another metric.
UPDATE chores SET metric_type='amount', metric_unit='mg', has_volume_ml=TRUE
WHERE (predefined_key IN ('Cat Meds','Baby Meds') OR lower(trim(name))='baby meds')
AND metric_type IN ('','none') AND NOT has_volume_ml;
-- Reuse an existing Baby Meds activity instead of creating a duplicate.
UPDATE chores c SET predefined_key='Baby Meds', is_predefined=TRUE
WHERE COALESCE(c.predefined_key,'')='' AND c.id=(SELECT min(existing.id) FROM chores existing WHERE existing.household_id=c.household_id AND lower(trim(existing.name))='baby meds' AND COALESCE(existing.predefined_key,'')='')
AND NOT EXISTS (SELECT 1 FROM chores existing WHERE existing.household_id=c.household_id AND existing.predefined_key='Baby Meds');
INSERT INTO chores (household_id,name,icon,color,sort_order,category,is_predefined,predefined_key,indicator_labels,indicator_defaults,has_volume_ml,metric_type,metric_unit)
SELECT h.id,'Baby Meds','💊','#A78BFA',15,'care',TRUE,'Baby Meds','[]','[]',TRUE,'amount','mg'
FROM households h WHERE NOT EXISTS (SELECT 1 FROM chores c WHERE c.household_id=h.id AND c.predefined_key='Baby Meds');
