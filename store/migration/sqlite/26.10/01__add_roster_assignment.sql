CREATE TABLE roster_assignment (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  duty_date TEXT NOT NULL,
  user_id INTEGER NOT NULL,
  UNIQUE(duty_date, user_id)
);

-- Roster Tracker has no private notes: every memo is readable by everyone.
UPDATE memo SET visibility = 'PUBLIC' WHERE visibility <> 'PUBLIC';
