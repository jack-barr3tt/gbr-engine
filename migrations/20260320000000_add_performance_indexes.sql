-- Performance indexes for /services endpoint

CREATE INDEX IF NOT EXISTS idx_schedule_signalling_id
  ON schedule(signalling_id);

CREATE INDEX IF NOT EXISTS idx_schedule_atoc_code
  ON schedule(atoc_code);

CREATE INDEX IF NOT EXISTS idx_schedule_end_date
  ON schedule(schedule_end_date);

ANALYZE schedule;
ANALYZE schedule_location;
