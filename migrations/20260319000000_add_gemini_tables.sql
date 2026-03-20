-- Gemini message and allocation storage for LINX Passenger Train Allocation and Consist (S506.001)

CREATE TABLE IF NOT EXISTS gemini_message (
  id SERIAL PRIMARY KEY,
  message_identifier TEXT UNIQUE NOT NULL,
  message_date_time TIMESTAMPTZ,
  toc VARCHAR(10),
  received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gemini_allocation (
  id SERIAL PRIMARY KEY,
  resource_group_id VARCHAR(20) NOT NULL,
  diagram_date DATE NOT NULL,
  operational_train_number VARCHAR(10) NOT NULL,
  core VARCHAR(30) NOT NULL,
  start_date DATE NOT NULL,
  company VARCHAR(10),
  allocation_sequence_number INT,
  train_origin_tiploc VARCHAR(15),
  train_dest_tiploc VARCHAR(15),
  allocation_origin_tiploc VARCHAR(15),
  allocation_dest_tiploc VARCHAR(15),
  resource_group_position VARCHAR(5),
  reversed VARCHAR(1),
  type_of_resource VARCHAR(1),
  message_identifier TEXT,
  message_date_time TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gemini_allocation_op_start_msgtime
  ON gemini_allocation (operational_train_number, start_date, message_date_time);

CREATE INDEX IF NOT EXISTS idx_gemini_allocation_core_start_msgtime
  ON gemini_allocation (core, start_date, message_date_time);

CREATE INDEX IF NOT EXISTS idx_gemini_allocation_op_start
  ON gemini_allocation (operational_train_number, start_date);

CREATE INDEX IF NOT EXISTS idx_gemini_allocation_core_start
  ON gemini_allocation (core, start_date);

