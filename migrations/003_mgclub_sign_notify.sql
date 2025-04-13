-- 创建用户通知设置表
CREATE TABLE IF NOT EXISTS mgclub_notify_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL UNIQUE,
    group_id INTEGER,  -- 群聊ID，如果是私聊则为null
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES mgclub_users(user_id) ON DELETE CASCADE
);

-- 创建更新时间触发器
CREATE TRIGGER IF NOT EXISTS update_mgclub_notify_settings_timestamp 
AFTER UPDATE ON mgclub_notify_settings
BEGIN
    UPDATE mgclub_notify_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END; 