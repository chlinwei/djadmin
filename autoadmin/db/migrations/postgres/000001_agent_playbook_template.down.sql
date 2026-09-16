-- Rollback: remove the Agent install playbook template seeded by 000001.
DELETE FROM automation_playbook_template WHERE category = 'agent';
