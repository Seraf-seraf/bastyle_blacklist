from pathlib import Path

from ai_vector_service.config import load_settings, parse_byte_size


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
