-- Skills: reusable instruction documents served to MCP clients as tools.
CREATE TABLE IF NOT EXISTS skills (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Skill permissions (users)
CREATE TABLE IF NOT EXISTS skill_allowed_users (
    skill_id VARCHAR(255) REFERENCES skills(id) ON DELETE CASCADE,
    user_email VARCHAR(255) NOT NULL,
    PRIMARY KEY (skill_id, user_email)
);

-- Skill permissions (groups)
CREATE TABLE IF NOT EXISTS skill_allowed_groups (
    skill_id VARCHAR(255) REFERENCES skills(id) ON DELETE CASCADE,
    group_name VARCHAR(255) NOT NULL,
    PRIMARY KEY (skill_id, group_name)
);
