ALTER TABLE custom_agents ADD COLUMN admin_users JSONB DEFAULT '[]';
ALTER TABLE custom_agents ADD COLUMN admin_groups JSONB DEFAULT '[]';
