from pathlib import Path

from ai_vector_service.config import load_settings, parse_byte_size, parse_duration_seconds


def test_parse_byte_size_accepts_human_readable_units():
    assert parse_byte_size("20MiB") == 20 * 1024 * 1024
    assert parse_byte_size("1MB") == 1000 * 1000
    assert parse_byte_size(8) == 8


def test_load_settings_reads_max_upload_bytes_as_size(tmp_path):
    config_path = tmp_path / "config.yaml"
    config_path.write_text(
        """
media_config:
  max_upload_bytes: 20MiB
""",
        encoding="utf-8",
    )

    settings = load_settings(Path(config_path))

    assert settings.max_upload_bytes == 20 * 1024 * 1024


def test_parse_duration_seconds_accepts_human_readable_units():
    assert parse_duration_seconds("500ms") == 0.5
    assert parse_duration_seconds("5s") == 5
    assert parse_duration_seconds("2m") == 120
    assert parse_duration_seconds(3) == 3


def test_load_settings_reads_database_settings(tmp_path):
    config_path = tmp_path / "config.yaml"
    config_path.write_text(
        """
database:
  dsn: "postgres://user:pass@db:5432/app?sslmode=disable"
  min_conns: 2
  max_conns: 12
  connect_timeout: 6s
matching:
  ai_vector:
    index_path: "faiss.index"
""",
        encoding="utf-8",
    )

    settings = load_settings(Path(config_path))

    assert settings.database.dsn == "postgres://user:pass@db:5432/app?sslmode=disable"
    assert settings.database.min_size == 2
    assert settings.database.max_size == 12
    assert settings.database.connect_timeout == 6
    assert settings.index_path == "faiss.index"
