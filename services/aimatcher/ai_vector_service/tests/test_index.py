from pathlib import Path

import pytest

from ai_vector_service.index import (
    INDEX_TYPE,
    FaissHNSWVectorIndex,
    HNSWConfig,
    IndexedVectorRef,
)
from ai_vector_service.storage import IndexState, VectorFrame


def test_faiss_hnsw_index_search_on_empty_index_returns_no_hits():
    index = FaissHNSWVectorIndex(dimension=3, config=_test_config())

    assert index.search([1.0, 0.0, 0.0], top_k=5) == []


def test_faiss_hnsw_index_finds_added_ban(tmp_path: Path):
    index = FaissHNSWVectorIndex(dimension=3, config=_test_config())
    store = MemoryVectorStore()
    ban = _insert_ban(
        store,
        file_unique_id="file-1",
        vectors=[
            VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0]),
            VectorFrame(frame_index=1, position_millis=100, vector=[0.0, 1.0, 0.0]),
        ],
    )

    index.add_ban(ban)
    hits = index.search([0.9, 0.1, 0.0], top_k=2)

    assert hits[0].ban_id == ban.id
    assert hits[0].frame_index == 0
    assert hits[0].score > 0.99


def test_faiss_hnsw_index_filters_added_bans_by_chat_id(tmp_path: Path):
    index = FaissHNSWVectorIndex(dimension=3, config=_test_config())
    store = MemoryVectorStore()
    first_ban = _insert_ban(
        store,
        chat_id=10,
        file_unique_id="file-1",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    second_ban = _insert_ban(
        store,
        chat_id=20,
        file_unique_id="file-2",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    index.add_bans([first_ban, second_ban])

    first_hits = index.search([1.0, 0.0, 0.0], top_k=5, chat_id=10)
    second_hits = index.search([1.0, 0.0, 0.0], top_k=5, chat_id=20)
    missing_hits = index.search([1.0, 0.0, 0.0], top_k=5, chat_id=30)

    assert index._ids_by_chat == {10: [0], 20: [1]}
    assert [hit.ban_id for hit in first_hits] == [first_ban.id]
    assert [hit.chat_id for hit in first_hits] == [10]
    assert [hit.ban_id for hit in second_hits] == [second_ban.id]
    assert [hit.chat_id for hit in second_hits] == [20]
    assert missing_hits == []


def test_faiss_hnsw_index_saves_and_loads_from_disk(tmp_path: Path):
    store = MemoryVectorStore()
    ban = _insert_ban(
        store,
        file_unique_id="file-1",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    index_path = tmp_path / "faiss-image.index"
    index = FaissHNSWVectorIndex.build(dimension=3, bans=[ban], config=_test_config())

    index.save(
        store=store,
        path=index_path,
        model_name="test-model",
        model_revision="test-revision",
    )
    state = store.load_index_state(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_type=INDEX_TYPE,
    )
    assert state is not None
    assert state.active_vectors_hash
    assert state.index_file_sha256

    loaded = FaissHNSWVectorIndex.load(
        path=index_path,
        dimension=3,
        refs=[
            IndexedVectorRef(
                ban_id=ban.id,
                chat_id=ban.chat_id,
                frame_index=ban.frames[0].frame_index,
            )
        ],
        index_file_sha256=state.index_file_sha256,
        config=_test_config(),
    )

    hits = loaded.search([1.0, 0.0, 0.0], top_k=1)

    assert hits[0].ban_id == ban.id
    assert hits[0].frame_index == 0
    assert hits[0].score > 0.99


def test_faiss_hnsw_index_load_or_rebuild_rebuilds_stale_index(tmp_path: Path):
    store = MemoryVectorStore()
    _insert_ban(
        store,
        file_unique_id="file-1",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    store.save_index_state(
        IndexState(
            model_name="test-model",
            model_revision="test-revision",
            vector_dim=3,
            index_type=INDEX_TYPE,
            index_path=str(tmp_path / "faiss-image.index"),
            active_vectors_count=999,
            active_vectors_hash="stale-active-vectors-hash",
            index_file_sha256="stale-index-file-sha256",
            rebuilt_at="2026-05-08T00:00:00Z",
        )
    )

    index = FaissHNSWVectorIndex.load_or_rebuild(
        store=store,
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_path=tmp_path / "faiss-image.index",
        config=_test_config(),
    )

    assert index.vectors_count == 1
    hits = index.search([1.0, 0.0, 0.0], top_k=1)
    assert hits[0].frame_index == 0
    assert hits[0].score > 0.99
    assert (tmp_path / "faiss-image.index").exists()
    state = store.load_index_state(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_type=INDEX_TYPE,
    )
    assert state is not None
    assert state.active_vectors_count == 1
    assert state.active_vectors_hash
    assert state.index_file_sha256


def test_faiss_hnsw_index_load_or_rebuild_rebuilds_when_active_bans_change_same_count(
    tmp_path: Path,
):
    store = MemoryVectorStore()
    old_ban = _insert_ban(
        store,
        file_unique_id="file-1",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    index_path = tmp_path / "faiss-image.index"
    index = FaissHNSWVectorIndex.build(dimension=3, bans=[old_ban], config=_test_config())
    index.save(
        store=store,
        path=index_path,
        model_name="test-model",
        model_revision="test-revision",
    )

    assert store.deactivate_ban(old_ban.id) is True
    new_ban = _insert_ban(
        store,
        file_unique_id="file-2",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[0.0, 1.0, 0.0])],
    )

    loaded = FaissHNSWVectorIndex.load_or_rebuild(
        store=store,
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_path=index_path,
        config=_test_config(),
    )

    new_hits = loaded.search([0.0, 1.0, 0.0], top_k=1)
    assert new_hits[0].ban_id == new_ban.id
    assert new_hits[0].score > 0.99

    old_hits = loaded.search([1.0, 0.0, 0.0], top_k=1)
    assert old_hits[0].ban_id == new_ban.id
    assert old_hits[0].score < 0.5


def test_faiss_hnsw_index_load_rejects_checksum_mismatch(tmp_path: Path):
    store = MemoryVectorStore()
    ban = _insert_ban(
        store,
        file_unique_id="file-1",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    index_path = tmp_path / "faiss-image.index"
    index = FaissHNSWVectorIndex.build(dimension=3, bans=[ban], config=_test_config())
    index.save(
        store=store,
        path=index_path,
        model_name="test-model",
        model_revision="test-revision",
    )

    with pytest.raises(ValueError, match="контрольная сумма"):
        FaissHNSWVectorIndex.load(
            path=index_path,
            dimension=3,
            refs=[
                IndexedVectorRef(
                    ban_id=ban.id,
                    chat_id=ban.chat_id,
                    frame_index=ban.frames[0].frame_index,
                )
            ],
            index_file_sha256="0" * 64,
            config=_test_config(),
        )


def test_faiss_hnsw_index_load_or_rebuild_rebuilds_checksum_mismatch(tmp_path: Path):
    store = MemoryVectorStore()
    ban = _insert_ban(
        store,
        file_unique_id="file-1",
        vectors=[VectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    index_path = tmp_path / "faiss-image.index"
    index = FaissHNSWVectorIndex.build(dimension=3, bans=[ban], config=_test_config())
    index.save(
        store=store,
        path=index_path,
        model_name="test-model",
        model_revision="test-revision",
    )
    index_path.write_bytes(b"not a faiss index")

    loaded = FaissHNSWVectorIndex.load_or_rebuild(
        store=store,
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        index_path=index_path,
        config=_test_config(),
    )

    hits = loaded.search([1.0, 0.0, 0.0], top_k=1)
    assert hits[0].ban_id == ban.id
    assert hits[0].score > 0.99


def _insert_ban(
    store,
    *,
    file_unique_id: str,
    vectors: list[VectorFrame],
    chat_id: int = 10,
):
    ban_id = store.insert_ban(
        chat_id=chat_id,
        file_unique_id=file_unique_id,
        media_type="photo",
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
        frames=vectors,
    )
    bans = store.load_active_bans(
        model_name="test-model",
        model_revision="test-revision",
        vector_dim=3,
    )
    return next(ban for ban in bans if ban.id == ban_id)


def _test_config() -> HNSWConfig:
    return HNSWConfig(m=8, ef_construction=20, ef_search=16)


class MemoryVectorStore:
    def __init__(self):
        self.bans = []
        self.states = {}
        self.next_ban_id = 0
        self.next_frame_id = 0

    def insert_ban(
        self,
        *,
        chat_id,
        file_unique_id,
        media_type,
        model_name,
        model_revision,
        vector_dim,
        frames,
    ):
        from ai_vector_service.storage import StoredVectorFrame, VectorBan

        self.next_ban_id += 1
        stored_frames = []
        for frame in frames:
            self.next_frame_id += 1
            stored_frames.append(
                StoredVectorFrame(
                    id=self.next_frame_id,
                    ban_id=self.next_ban_id,
                    frame_index=frame.frame_index,
                    position_millis=frame.position_millis,
                    vector=frame.vector,
                )
            )
        self.bans.append(
            VectorBan(
                id=self.next_ban_id,
                chat_id=chat_id,
                file_unique_id=file_unique_id,
                media_type=media_type,
                model_name=model_name,
                model_revision=model_revision,
                vector_dim=vector_dim,
                frames_count=len(frames),
                active=True,
                created_at="2026-05-19T00:00:00Z",
                frames=stored_frames,
            )
        )
        return self.next_ban_id

    def load_active_bans(self, *, chat_id=None, model_name=None, model_revision=None, vector_dim=None):
        bans = [ban for ban in self.bans if ban.active]
        if chat_id is not None:
            bans = [ban for ban in bans if ban.chat_id == chat_id]
        if model_name is not None:
            bans = [ban for ban in bans if ban.model_name == model_name]
        if model_revision is not None:
            bans = [ban for ban in bans if ban.model_revision == model_revision]
        if vector_dim is not None:
            bans = [ban for ban in bans if ban.vector_dim == vector_dim]
        return bans

    def deactivate_ban(self, ban_id):
        from dataclasses import replace

        for index, ban in enumerate(self.bans):
            if ban.id == ban_id and ban.active:
                self.bans[index] = replace(ban, active=False)
                return True
        return False

    def save_index_state(self, state):
        self.states[(state.model_name, state.model_revision, state.vector_dim, state.index_type)] = state

    def load_index_state(self, *, model_name, model_revision, vector_dim, index_type):
        return self.states.get((model_name, model_revision, vector_dim, index_type))
