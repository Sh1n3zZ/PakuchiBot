-- 创建用户表
CREATE TABLE IF NOT EXISTS mgclub_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL UNIQUE,
    token TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 创建更新时间触发器
CREATE TRIGGER IF NOT EXISTS update_mgclub_users_timestamp 
AFTER UPDATE ON mgclub_users 
BEGIN
    UPDATE mgclub_users SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;
