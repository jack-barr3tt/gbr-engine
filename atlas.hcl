env "local" {
  src = "file://schema.sql"
  url = "postgres://${getenv("POSTGRES_USER")}:${getenv("POSTGRES_PASSWORD")}@localhost:5432/gbr_engine?sslmode=disable"
  dev = "postgres://${getenv("POSTGRES_USER")}:${getenv("POSTGRES_PASSWORD")}@localhost:5432/gbr_engine_atlas_dev?sslmode=disable"

  migration {
    dir = "file://migrations"
  }
}

