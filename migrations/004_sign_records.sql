-- 创建签到记录表
CREATE TABLE IF NOT EXISTS sign_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    sign_date DATE NOT NULL,
    status INTEGER NOT NULL DEFAULT 0, -- 0: 未签到, 1: 已签到, 2: 签到失败
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_retry_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, sign_date)
);

-- 创建更新时间触发器
CREATE TRIGGER IF NOT EXISTS update_sign_records_timestamp 
AFTER UPDATE ON sign_records
BEGIN
    UPDATE sign_records SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END; 