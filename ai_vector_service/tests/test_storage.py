from pathlib import Path
import sqlite3

import pytest

from ai_vector_service.storage import IndexState, SQLiteVectorStore, VectorFrame


def test_sqlite_vector_store_persists_active_bans(tmp_path: Path):
    store = _new_store(tmp_path)

    ban_id = store.insert_ban(
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
    assert bans[0].file_unique_id == "file-unique-id"
    assert bans[0].media_type == "animation"
    assert bans[0].model_name == "test-model"
    assert bans[0].model_revision == "test-revision"
    assert bans[0].vector_dim == 3
    assert bans[0].frames_count == 2
    assert bans[0].active is True
    assert [frame.frame_index for frame in bans[0].frames] == [0, 1]
    assert bans[0].frames[0].position_millis == 0
    assert bans[0].frames[0].vector == [1.0, 0.0, 0.0]
    assert bans[0].frames[1].position_millis == 500
    assert bans[0].frames[1].vector == [0.0, 1.0, 0.0]


def test_sqlite_vector_store_loads_after_reopen(tmp_path: Path):
    db_path = tmp_path / "vectors.sqlite"
    first = SQLiteVectorStore(db_path)
    first.insert_ban(
        file_unique_id="file-unique-id",
        media_type="photo",
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=2,
        frames=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0])],
    )
    first.close()

    second = SQLiteVectorStore(db_path)
    bans = second.load_active_bans(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=2,
    )

    assert len(bans) == 1
    assert bans[0].file_unique_id == "file-unique-id"
    assert bans[0].frames[0].vector == [1.0, 0.0]


def test_sqlite_vector_store_deactivates_ban(tmp_path: Path):
    store = _new_store(tmp_path)
    ban_id = store.insert_ban(
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


def test_sqlite_vector_store_filters_by_model_revision_and_dimension(tmp_path: Path):
    store = _new_store(tmp_path)
    store.insert_ban(
        file_unique_id="first-file",
        media_type="photo",
        model_name="test-model",
        model_revision="current",
        vector_dim=2,
        frames=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0])],
    )
    store.insert_ban(
        file_unique_id="second-file",
        media_type="photo",
        model_name="test-model",
        model_revision="old",
        vector_dim=2,
        frames=[VectorFrame(frame_index=0, position_millis=0, vector=[0.0, 1.0])],
    )

    bans = store.load_active_bans(
        model_name="test-model",
        model_revision="current",
        vector_dim=2,
    )

    assert len(bans) == 1
    assert bans[0].file_unique_id == "first-file"


def test_sqlite_vector_store_saves_index_state(tmp_path: Path):
    store = _new_store(tmp_path)
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

    assert loaded == state

    updated = IndexState(
        model_name=state.model_name,
        model_revision=state.model_revision,
        vector_dim=state.vector_dim,
        index_type=state.index_type,
        index_path=state.index_path,
        active_vectors_count=12,
        active_vectors_hash="updated-active-vectors-hash",
        index_file_sha256="updated-index-file-sha256",
        rebuilt_at="2026-05-08T11:00:00Z",
    )
    store.save_index_state(updated)

    assert store.load_index_state(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_type="hnsw-flat-ip",
    ) == updated


def test_sqlite_vector_store_migrates_index_state_integrity_columns(tmp_path: Path):
    db_path = tmp_path / "vectors.sqlite"
    with sqlite3.connect(db_path) as db:
        db.executescript(
            """
CREATE TABLE ai_vector_index_state (
    model_name TEXT NOT NULL,
    model_revision TEXT NOT NULL,
    vector_dim INTEGER NOT NULL,
    index_type TEXT NOT NULL,
    index_path TEXT NOT NULL,
    active_vectors_count INTEGER NOT NULL,
    rebuilt_at TEXT NOT NULL,
    PRIMARY KEY (model_name, model_revision, vector_dim, index_type)
);

INSERT INTO ai_vector_index_state (
    model_name, model_revision, vector_dim, index_type, index_path,
    active_vectors_count, rebuilt_at
)
VALUES (
    'test-model', 'test-revision', 3, 'hnsw-flat-ip',
    '/var/lib/bastyle/faiss-image.index', 10, '2026-05-08T10:00:00Z'
);
"""
        )

    store = SQLiteVectorStore(db_path)
    state = store.load_index_state(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_type="hnsw-flat-ip",
    )

    assert state is not None
    assert state.active_vectors_hash == ""
    assert state.index_file_sha256 == ""


def test_sqlite_vector_store_rejects_invalid_vectors(tmp_path: Path):
    store = _new_store(tmp_path)

    with pytest.raises(ValueError, match="vector dimension mismatch"):
        store.insert_ban(
            file_unique_id="file-unique-id",
            media_type="photo",
            model_name="test-model",
            model_revision="test-revision",
            vector_dim=3,
            frames=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0])],
        )


def _new_store(tmp_path: Path) -> SQLiteVectorStore:
    store = SQLiteVectorStore(tmp_path / "vectors.sqlite")
    return store
