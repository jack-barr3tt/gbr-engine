-- BPLAN (Public Interface Format) reference and network data
-- Migration: add bplan_* tables for bplan-loader.

CREATE TABLE IF NOT EXISTS bplan_pif (
  id SERIAL PRIMARY KEY,
  version VARCHAR(10) NOT NULL,
  source VARCHAR(50) NOT NULL,
  toc VARCHAR(10) NOT NULL,
  timetable_start TEXT NOT NULL,
  timetable_end TEXT NOT NULL,
  cycle_type VARCHAR(1) NOT NULL,
  cycle_indicator VARCHAR(1) NOT NULL,
  file_creation_time TEXT NOT NULL,
  sequence VARCHAR(10) NOT NULL
);

CREATE TABLE IF NOT EXISTS bplan_ref (
  id SERIAL PRIMARY KEY,
  category VARCHAR(10) NOT NULL,
  subcode VARCHAR(50) NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_bplan_ref_category ON bplan_ref(category);

CREATE TABLE IF NOT EXISTS bplan_loc (
  tiploc VARCHAR(15) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  start_date TEXT NOT NULL,
  end_date TEXT,
  x_coord VARCHAR(20) NOT NULL DEFAULT '',
  y_coord VARCHAR(20) NOT NULL DEFAULT '',
  optionality VARCHAR(1) NOT NULL,
  zone VARCHAR(2) NOT NULL,
  stanox VARCHAR(10),
  stanox_flag VARCHAR(1),
  extra VARCHAR(10)
);
CREATE INDEX IF NOT EXISTS idx_bplan_loc_zone ON bplan_loc(zone);
CREATE INDEX IF NOT EXISTS idx_bplan_loc_stanox ON bplan_loc(stanox) WHERE stanox IS NOT NULL AND stanox != '';

CREATE TABLE IF NOT EXISTS bplan_tld (
  id SERIAL PRIMARY KEY,
  load_code VARCHAR(20) NOT NULL,
  load_subcode VARCHAR(20) NOT NULL DEFAULT '',
  speed VARCHAR(10) NOT NULL DEFAULT '',
  reserved VARCHAR(10) NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  train_type VARCHAR(10) NOT NULL DEFAULT '',
  sub_type VARCHAR(10) NOT NULL DEFAULT '',
  speed_or_id VARCHAR(10) NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_bplan_tld_load_code ON bplan_tld(load_code);

CREATE TABLE IF NOT EXISTS bplan_plt (
  id SERIAL PRIMARY KEY,
  tiploc VARCHAR(15) NOT NULL,
  platform_id VARCHAR(10) NOT NULL,
  start_date TEXT NOT NULL,
  end_date TEXT,
  platform_num VARCHAR(10),
  reserved VARCHAR(10),
  flag VARCHAR(1),
  public_flag VARCHAR(1) NOT NULL,
  UNIQUE(tiploc, platform_id)
);
CREATE INDEX IF NOT EXISTS idx_bplan_plt_tiploc ON bplan_plt(tiploc);

CREATE TABLE IF NOT EXISTS bplan_nwk (
  id SERIAL PRIMARY KEY,
  from_tiploc VARCHAR(15) NOT NULL,
  to_tiploc VARCHAR(15) NOT NULL,
  link_type TEXT,
  reserved TEXT,
  start_date TEXT NOT NULL,
  end_date TEXT,
  direction VARCHAR(1) NOT NULL,
  direction2 VARCHAR(1) NOT NULL,
  distance VARCHAR(20) NOT NULL DEFAULT '',
  flag1 VARCHAR(1),
  flag2 VARCHAR(1),
  flag3 VARCHAR(1),
  zone VARCHAR(2),
  flag4 VARCHAR(1),
  reserved2 TEXT,
  extra TEXT,
  reserved3 TEXT
);
CREATE INDEX IF NOT EXISTS idx_bplan_nwk_from_tiploc ON bplan_nwk(from_tiploc);
CREATE INDEX IF NOT EXISTS idx_bplan_nwk_to_tiploc ON bplan_nwk(to_tiploc);
CREATE INDEX IF NOT EXISTS idx_bplan_nwk_from_to ON bplan_nwk(from_tiploc, to_tiploc);

CREATE TABLE IF NOT EXISTS bplan_tlk (
  id SERIAL PRIMARY KEY,
  from_tiploc VARCHAR(15) NOT NULL,
  to_tiploc VARCHAR(15) NOT NULL,
  route_link_type VARCHAR(20),
  timing_load VARCHAR(20),
  sub_type VARCHAR(20),
  speed VARCHAR(10),
  load_variant VARCHAR(10),
  penalty1 VARCHAR(10),
  penalty2 VARCHAR(10),
  start_date TEXT NOT NULL,
  end_date TEXT,
  run_time VARCHAR(20) NOT NULL,
  reserved VARCHAR(10)
);
CREATE INDEX IF NOT EXISTS idx_bplan_tlk_from_tiploc ON bplan_tlk(from_tiploc);
CREATE INDEX IF NOT EXISTS idx_bplan_tlk_to_tiploc ON bplan_tlk(to_tiploc);
CREATE INDEX IF NOT EXISTS idx_bplan_tlk_from_to ON bplan_tlk(from_tiploc, to_tiploc);
CREATE INDEX IF NOT EXISTS idx_bplan_tlk_from_to_load ON bplan_tlk(from_tiploc, to_tiploc, timing_load);
