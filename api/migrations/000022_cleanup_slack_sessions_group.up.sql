DELETE FROM agent_group_sessions WHERE group_id = 'slack-sessions';
DELETE FROM agent_group_allowed_users WHERE group_id = 'slack-sessions';
DELETE FROM agent_group_allowed_groups WHERE group_id = 'slack-sessions';
DELETE FROM agent_groups WHERE id = 'slack-sessions';
