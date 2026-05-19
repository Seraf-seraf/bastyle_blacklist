from pathlib import Path

import pytest
from psycopg import DataError, connect
from testcontainers.postgres import PostgresContainer

from ai_vector_service.config import DatabaseSettings
from ai_vector_service.db import create_pool
from ai_vector_service.storage import IndexState, PostgresVectorStore, VectorFrame


@pytest.fixture(scope="module")
def postgres_dsn():
    with PostgresContainer("postgres:16-alpine", username="bastyle", password="bastyle", dbname="bastyle") as postgres:
        dsn = postgres.get_connection_url(driver=None)
        _apply_postgres_migrations(dsn)
        yield dsn


@pytest.fixture()
def store(postgres_dsn):
    _truncate_tables(postgres_dsn)
    pool = create_pool(DatabaseSettings(dsn=postgres_dsn, min_size=1, max_size=2, connect_timeout=5))
    try:
        yield PostgresVectorStore(pool)
    finally:
        pool.close()


def test_postgres_vector_store_persists_active_bans(store: PostgresVectorStore):
    ban_id = store.insert_ban(
        chat_id=10,
        file_unique_id="file-unique-id",
        media_type="animation",
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        frames=[
            VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0]),
            VectorFrame(frame_index=1, position_millis=500, vector=[0.0, 1.0, 0.0]),
        ],
    )

    bans = store.load_active_bans(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
    )

    assert len(bans) == 1
    assert bans[0].id == ban_id
    assert bans[0].chat_id == 10
    assert bans[0].file_unique_id == "file-unique-id"
    assert bans[0].media_type == "animation"
    assert bans[0].frames_count == 2
    assert [frame.frame_index for frame in bans[0].frames] == [0, 1]
    assert bans[0].frames[0].vector == [1.0, 0.0, 0.0]
    assert bans[0].frames[1].vector == [0.0, 1.0, 0.0]


def test_postgres_vector_store_loads_after_pool_reopen(postgres_dsn):
    _truncate_tables(postgres_dsn)
    first_pool = create_pool(DatabaseSettings(dsn=postgres_dsn, min_size=1, max_size=2, connect_timeout=5))
    first = PostgresVectorStore(first_pool)
    first.insert_ban(
        chat_id=10,
        file_unique_id="file-unique-id",
        media_type="photo",
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=2,
        frames=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0])],
    )
    first_pool.close()

    second_pool = create_pool(DatabaseSettings(dsn=postgres_dsn, min_size=1, max_size=2, connect_timeout=5))
    try:
        second = PostgresVectorStore(second_pool)
        bans = second.load_active_bans(
            model_name="test-model",
            model_revision="test-revision",
            vector_dim=2,
        )
    finally:
        second_pool.close()

    assert len(bans) == 1
    assert bans[0].file_unique_id == "file-unique-id"
    assert bans[0].frames[0].vector == [1.0, 0.0]


def test_postgres_vector_store_deactivates_ban(store: PostgresVectorStore):
    ban_id = store.insert_ban(
        chat_id=10,
        file_unique_id="file-unique-id",
        media_type="photo",
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=2,
        frames=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0])],
    )

    assert store.active_vectors_count(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=2,
    ) == 1
    assert store.deactivate_ban(ban_id) is True
    assert store.deactivate_ban(ban_id) is False
    assert store.load_active_bans(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=2,
    ) == []
    assert store.active_vectors_count(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=2,
    ) == 0


def test_postgres_vector_store_filters_by_chat_model_revision_and_dimension(store: PostgresVectorStore):
    store.insert_ban(
        chat_id=10,
        file_unique_id="first-file",
        media_type="photo",
        model_name="test-model",
        model_revision="current",
        vector_dim=2,
        frames=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0])],
    )
    store.insert_ban(
        chat_id=20,
        file_unique_id="second-file",
        media_type="photo",
        model_name="test-model",
        model_revision="old",
        vector_dim=2,
        frames=[VectorFrame(frame_index=0, position_millis=0, vector=[0.0, 1.0])],
    )

    bans = store.load_active_bans(
        chat_id=10,
        model_name="test-model",
        model_revision="current",
        vector_dim=2,
    )

    assert len(bans) == 1
    assert bans[0].file_unique_id == "first-file"
    assert store.active_vectors_count(
        chat_id=10,
        model_name="test-model",
        model_revision="current",
        vector_dim=2,
    ) == 1


def test_postgres_vector_store_saves_index_state(store: PostgresVectorStore):
    state = IndexState(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_type="hnsw-flat-ip",
        index_path="/var/lib/bastyle/faiss-image.index",
        active_vectors_count=10,
        active_vectors_hash="active-vectors-hash",
        index_file_sha256="index-file-sha256",
        rebuilt_at="2026-05-08T10:00:00Z",
    )

    store.save_index_state(state)
    loaded = store.load_index_state(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_type="hnsw-flat-ip",
    )

    assert loaded is not None
    assert loaded.active_vectors_count == 10
    assert loaded.active_vectors_hash == "active-vectors-hash"
    assert loaded.index_file_sha256 == "index-file-sha256"


def test_postgres_vector_store_rejects_invalid_vectors(store: PostgresVectorStore):
    with pytest.raises(ValueError, match="размерность вектора не совпадает"):
        store.insert_ban(
            chat_id=10,
            file_unique_id="file-unique-id",
            media_type="photo",
            model_name="test-model",
            model_revision="test-revision",
            vector_dim=3,
            frames=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0])],
        )


def test_postgres_schema_rejects_vector_blob_with_wrong_dimension(postgres_dsn):
    _truncate_tables(postgres_dsn)
    with connect(postgres_dsn) as conn:
        ban_uid = "00000000-0000-0000-0000-000000000001"
        conn.execute(
            """
INSERT INTO media_ban (ban_uid, chat_id, media_type, file_unique_id)
VALUES (%s, %s, %s, %s)
""",
            (ban_uid, 10, "photo", "file-unique-id"),
        )
        ban_id = conn.execute(
            """
INSERT INTO ai_vector_ban (
    ban_uid, chat_id, file_unique_id, media_type, model_name, model_revision,
    vector_dim, frames_count, vector_signature
)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
RETURNING id
""",
            (ban_uid, 10, "file-unique-id", "photo", "test-model", "test-revision", 3, 1, "signature"),
        ).fetchone()[0]

        with pytest.raises(DataError):
            conn.execute(
                """
INSERT INTO ai_vector_frame (ban_id, frame_index, position_millis, vector_blob)
VALUES (%s, %s, %s, %s)
""",
                (ban_id, 0, 0, b"\x00\x00\x80?"),
            )


def _apply_postgres_migrations(dsn: str) -> None:
    repo_root = Path(__file__).parents[4]
    migrations_dir = repo_root / "services/bot/internal/adapters/database/postgres/migrations"
    with connect(dsn) as conn:
        for path in sorted(migrations_dir.glob("*.sql")):
            sql = _goose_up_sql(path)
            if sql:
                conn.execute(sql)


def _goose_up_sql(path: Path) -> str:
    raw = path.read_text(encoding="utf-8")
    up = raw.split("-- +goose Down", 1)[0]
    return up.replace("-- +goose Up", "").strip()


def _truncate_tables(dsn: str) -> None:
    with connect(dsn) as conn:
        conn.execute(
            """
TRUNCATE TABLE
    blocked_exact,
    blocked_image_hash,
    blocked_image,
    blocked_video_like_frame_hash,
    blocked_video_like,
    ai_vector_frame,
    ai_vector_ban,
    ai_vector_index_state,
    outbox_events,
    index_checkpoints,
    media_ban
RESTART IDENTITY CASCADE
"""
        )
