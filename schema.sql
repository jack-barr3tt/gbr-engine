CREATE TABLE IF NOT EXISTS reference_toc (
  code VARCHAR(3) PRIMARY KEY,
  name VARCHAR(255) NOT NULL
);
CREATE TABLE IF NOT EXISTS reference_station (
  crs VARCHAR(3) PRIMARY KEY,
  name VARCHAR(255) NOT NULL
);
CREATE TABLE IF NOT EXISTS reference_fetch (
  key VARCHAR(255) PRIMARY KEY,
  last_fetched TIMESTAMP NOT NULL,
  max_age INTERVAL NOT NULL DEFAULT '24 hours'
);
INSERT INTO reference_fetch (key, last_fetched, max_age)
VALUES ('toc', '2000-01-01 00:00:00', '1 week'),
  ('stations', '2000-01-01 00:00:00', '1 week');