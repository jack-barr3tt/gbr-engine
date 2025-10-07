CREATE TABLE IF NOT EXISTS reference_toc (
  code VARCHAR(3) PRIMARY KEY,
  name VARCHAR(255) NOT NULL
);
CREATE TABLE IF NOT EXISTS tiploc (
  tiploc_code VARCHAR(15) PRIMARY KEY,
  nalco VARCHAR(6) NOT NULL,
  stanox VARCHAR(5),
  crs_code VARCHAR(3),
  description VARCHAR(255),
  tps_description VARCHAR(255) NOT NULL
);
CREATE TABLE IF NOT EXISTS schedule (
  id SERIAL PRIMARY KEY,
  train_uid VARCHAR(6) NOT NULL,
  transaction_type VARCHAR(10) NOT NULL,
  stp_indicator VARCHAR(1) NOT NULL,
  bank_holiday_running VARCHAR(1),
  applicable_timetable VARCHAR(1),
  atoc_code VARCHAR(3),
  schedule_days_runs VARCHAR(7) NOT NULL,
  schedule_start_date DATE NOT NULL,
  schedule_end_date DATE NOT NULL,
  train_status VARCHAR(1) NOT NULL,
  signalling_id VARCHAR(10) NOT NULL,
  train_category VARCHAR(2) NOT NULL,
  headcode VARCHAR(4) NOT NULL,
  course_indicator INT NOT NULL,
  train_service_code VARCHAR(8) NOT NULL,
  business_sector VARCHAR(2),
  power_type VARCHAR(3),
  timing_load VARCHAR(4),
  speed VARCHAR(3),
  operating_characteristics VARCHAR(10),
  train_class VARCHAR(1),
  sleepers VARCHAR(1),
  reservations VARCHAR(1),
  connection_indicator VARCHAR(1),
  catering_code VARCHAR(4),
  service_branding VARCHAR(20),
  traction_class VARCHAR(2),
  uic_code VARCHAR(5)
);
CREATE TABLE IF NOT EXISTS schedule_location (
  id SERIAL PRIMARY KEY,
  schedule_id INT NOT NULL REFERENCES schedule(id) ON DELETE CASCADE,
  location_type VARCHAR(2) NOT NULL,
  record_identity VARCHAR(2) NOT NULL,
  tiploc_code VARCHAR(15) NOT NULL,
  tiploc_instance VARCHAR(1),
  arrival TIME,
  public_arrival TIME,
  departure TIME,
  public_departure TIME,
  pass TIME,
  platform VARCHAR(10),
  line VARCHAR(3),
  path VARCHAR(3),
  engineering_allowance VARCHAR(2),
  pathing_allowance VARCHAR(2),
  performance_allowance VARCHAR(2),
  location_order INT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_schedule_location_schedule_id ON schedule_location (schedule_id);
CREATE INDEX IF NOT EXISTS idx_schedule_location_tiploc ON schedule_location (tiploc_code);
CREATE TABLE IF NOT EXISTS association (
  id SERIAL PRIMARY KEY,
  transaction_type VARCHAR(10) NOT NULL,
  main_train_uid VARCHAR(6) NOT NULL,
  assoc_train_uid VARCHAR(6) NOT NULL,
  assoc_start_date DATE NOT NULL,
  assoc_end_date DATE NOT NULL,
  assoc_days VARCHAR(7) NOT NULL,
  category VARCHAR(2) NOT NULL,
  date_indicator VARCHAR(1) NOT NULL,
  location VARCHAR(15) NOT NULL,
  base_location_suffix VARCHAR(1),
  assoc_location_suffix VARCHAR(1),
  diagram_type VARCHAR(1) NOT NULL,
  stp_indicator VARCHAR(1) NOT NULL,
  UNIQUE(
    main_train_uid,
    assoc_train_uid,
    assoc_start_date,
    location,
    stp_indicator
  )
);
CREATE TABLE IF NOT EXISTS reference_fetch (
  key VARCHAR(255) PRIMARY KEY,
  last_fetched TIMESTAMP NOT NULL,
  max_age INTERVAL NOT NULL DEFAULT '24 hours'
);
INSERT INTO reference_fetch (key, last_fetched, max_age)
VALUES ('toc', '2000-01-01 00:00:00', '1 week');