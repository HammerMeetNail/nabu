-- Member access suspension ("disable" a household member without removing them).
-- A suspended membership row keeps its role and history but grants no access
-- until an owner/admin re-enables it. NULL suspended_at means active.
ALTER TABLE user_households ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ;
