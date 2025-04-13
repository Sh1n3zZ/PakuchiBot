-- 创建用户人品表
CREATE TABLE IF NOT EXISTS user_luck (
    user_id TEXT NOT NULL,
    day DATE NOT NULL,
    value INTEGER CHECK(value BETWEEN 0 AND 100),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, day)
);
