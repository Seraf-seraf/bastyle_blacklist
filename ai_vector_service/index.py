from dataclasses import dataclass
from datetime import UTC, datetime
import hashlib
from pathlib import Path
import threading

import numpy as np

from ai_vector_service.storage import IndexState, SQLiteVectorStore, VectorBan


INDEX_TYPE = "hnsw-flat-ip"


@dataclass(frozen=True)
class HNSWConfig:
    m: int = 32
    ef_construction: int = 80
    ef_search: int = 64


@dataclass(frozen=True)
class VectorSearchHit:
    ban_id: int
    frame_index: int
    score: float


@dataclass(frozen=True)
class IndexedVectorRef:
    ban_id: int
    frame_index: int


class FaissHNSWVectorIndex:
    def __init__(
        self,
        *,
        dimension: int,
        config: HNSWConfig | None = None,
        index: object | None = None,
        refs: list[IndexedVectorRef] | None = None,
    ) -> None:
        if dimension <= 0:
            raise ValueError("размерность вектора должна быть положительной")

        self._faiss = _load_faiss()
        self._dimension = dimension
        self._config = config or HNSWConfig()
        self._validate_config(self._config)
        self._index = index or self._new_index()
        self._refs = refs or []
        self._lock = threading.RLock()

        if self._index.d != dimension:
            raise ValueError("размерность индекса faiss не совпадает")
        if self._index.ntotal != len(self._refs):
            raise ValueError("количество записей индекса faiss и ссылок не совпадает")

    @property
    def dimension(self) -> int:
        return self._dimension

    @property
    def vectors_count(self) -> int:
        with self._lock:
            return int(self._index.ntotal)

    @classmethod
    def build(
        cls,
        *,
        dimension: int,
        bans: list[VectorBan],
        config: HNSWConfig | None = None,
    ) -> "FaissHNSWVectorIndex":
        vector_index = cls(dimension=dimension, config=config)
        vector_index.add_bans(bans)
        return vector_index

    @classmethod
    def load_or_rebuild(
        cls,
        *,
        store: SQLiteVectorStore,
        model_name: str,
        model_revision: str,
        vector_dim: int,
        index_path: str | Path,
        config: HNSWConfig | None = None,
    ) -> "FaissHNSWVectorIndex":
        path = Path(index_path)
        active_bans = store.load_active_bans(
            model_name=model_name,
            model_revision=model_revision,
            vector_dim=vector_dim,
        )
        refs = _bans_to_refs(active_bans)
        active_vectors_hash = _active_vectors_hash(active_bans)
        state = store.load_index_state(
            model_name=model_name,
            model_revision=model_revision,
            vector_dim=vector_dim,
            index_type=INDEX_TYPE,
        )

        if (
            state is not None
            and _state_matches(state, path, len(refs), active_vectors_hash)
            and path.exists()
        ):
            try:
                return cls.load(
                    path=path,
                    dimension=vector_dim,
                    refs=refs,
                    index_file_sha256=state.index_file_sha256,
                    config=config,
                )
            except RuntimeError:
                pass
            except ValueError:
                pass

        rebuilt = cls.build(dimension=vector_dim, bans=active_bans, config=config)
        rebuilt.save(
            store=store,
            path=path,
            model_name=model_name,
            model_revision=model_revision,
        )
        return rebuilt

    @classmethod
    def load(
        cls,
        *,
        path: str | Path,
        dimension: int,
        refs: list[IndexedVectorRef],
        index_file_sha256: str,
        config: HNSWConfig | None = None,
    ) -> "FaissHNSWVectorIndex":
        if not index_file_sha256:
            raise ValueError("контрольная сумма файла индекса обязательна")

        faiss = _load_faiss()
        index = _read_verified_index(faiss, Path(path), index_file_sha256)
        if hasattr(index, "hnsw"):
            index.hnsw.efSearch = (config or HNSWConfig()).ef_search

        return cls(dimension=dimension, config=config, index=index, refs=refs)

    def add_ban(self, ban: VectorBan) -> None:
        self.add_bans([ban])

    def validate_vectors(self, vectors: list[list[float]]) -> None:
        _normalize_vectors(vectors, self._dimension)

    def add_bans(self, bans: list[VectorBan]) -> None:
        vectors: list[list[float]] = []
        refs: list[IndexedVectorRef] = []
        for ban in bans:
            if ban.vector_dim != self._dimension:
                raise ValueError("размерность вектора бана не совпадает")
            for frame in ban.frames:
                vectors.append(frame.vector)
                refs.append(IndexedVectorRef(ban_id=ban.id, frame_index=frame.frame_index))

        if not vectors:
            return

        matrix = _normalize_vectors(vectors, self._dimension)
        with self._lock:
            self._index.add(matrix)
            self._refs.extend(refs)

    def search(self, vector: list[float], top_k: int) -> list[VectorSearchHit]:
        if top_k <= 0:
            raise ValueError("top_k должен быть положительным")

        query = _normalize_vectors([vector], self._dimension)
        with self._lock:
            if self._index.ntotal == 0:
                return []

            scores, indices = self._index.search(query, top_k)
            hits: list[VectorSearchHit] = []
            for score, index_id in zip(scores[0], indices[0]):
                if index_id < 0:
                    continue
                ref = self._refs[int(index_id)]
                hits.append(
                    VectorSearchHit(
                        ban_id=ref.ban_id,
                        frame_index=ref.frame_index,
                        score=float(score),
                    )
                )

        return hits

    def save(
        self,
        *,
        store: SQLiteVectorStore,
        path: str | Path,
        model_name: str,
        model_revision: str,
    ) -> None:
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        active_bans = store.load_active_bans(
            model_name=model_name,
            model_revision=model_revision,
            vector_dim=self._dimension,
        )
        refs = _bans_to_refs(active_bans)
        active_vectors_hash = _active_vectors_hash(active_bans)

        with self._lock:
            if int(self._index.ntotal) != len(refs) or self._refs != refs:
                raise ValueError("индекс faiss не соответствует активным банам")

            self._faiss.write_index(self._index, str(path))
            vectors_count = int(self._index.ntotal)

        index_file_sha256 = _file_sha256(path)
        store.save_index_state(
            IndexState(
                model_name=model_name,
                model_revision=model_revision,
                vector_dim=self._dimension,
                index_type=INDEX_TYPE,
                index_path=str(path),
                active_vectors_count=vectors_count,
                active_vectors_hash=active_vectors_hash,
                index_file_sha256=index_file_sha256,
                rebuilt_at=datetime.now(UTC).isoformat(),
            )
        )

    def _new_index(self):
        index = self._faiss.IndexHNSWFlat(
            self._dimension,
            self._config.m,
            self._faiss.METRIC_INNER_PRODUCT,
        )
        index.hnsw.efConstruction = self._config.ef_construction
        index.hnsw.efSearch = self._config.ef_search
        return index

    @staticmethod
    def _validate_config(config: HNSWConfig) -> None:
        if config.m <= 0:
            raise ValueError("параметр hnsw m должен быть положительным")
        if config.ef_construction <= 0:
            raise ValueError("параметр hnsw ef_construction должен быть положительным")
        if config.ef_search <= 0:
            raise ValueError("параметр hnsw ef_search должен быть положительным")


def _bans_to_refs(bans: list[VectorBan]) -> list[IndexedVectorRef]:
    refs: list[IndexedVectorRef] = []
    for ban in bans:
        for frame in ban.frames:
            refs.append(IndexedVectorRef(ban_id=ban.id, frame_index=frame.frame_index))
    return refs


def _state_matches(
    state: IndexState,
    path: Path,
    active_vectors_count: int,
    active_vectors_hash: str,
) -> bool:
    return (
        state.index_path == str(path)
        and state.active_vectors_count == active_vectors_count
        and state.active_vectors_hash == active_vectors_hash
        and bool(state.index_file_sha256)
        and state.index_type == INDEX_TYPE
    )


def _active_vectors_hash(bans: list[VectorBan]) -> str:
    digest = hashlib.sha256()
    for ban in bans:
        _hash_int(digest, ban.id)
        _hash_text(digest, ban.file_unique_id)
        _hash_text(digest, ban.media_type)
        _hash_text(digest, ban.model_name)
        _hash_text(digest, ban.model_revision)
        _hash_int(digest, ban.vector_dim)
        _hash_int(digest, ban.frames_count)

        for frame in ban.frames:
            _hash_int(digest, frame.id)
            _hash_int(digest, frame.ban_id)
            _hash_int(digest, frame.frame_index)
            _hash_int(digest, frame.position_millis)
            vector = np.asarray(frame.vector, dtype=np.float32)
            _hash_int(digest, int(vector.size))
            digest.update(vector.tobytes())

    return digest.hexdigest()


def _hash_int(digest, value: int) -> None:
    digest.update(value.to_bytes(8, byteorder="big", signed=True))


def _hash_text(digest, value: str) -> None:
    encoded = value.encode("utf-8")
    _hash_int(digest, len(encoded))
    digest.update(encoded)


def _file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as file:
        for chunk in iter(lambda: file.read(1024 * 1024), b""):
            digest.update(chunk)

    return digest.hexdigest()


def _read_verified_index(faiss, path: Path, expected_sha256: str):
    index_bytes = path.read_bytes()
    actual_sha256 = hashlib.sha256(index_bytes).hexdigest()
    if actual_sha256 != expected_sha256:
        raise ValueError("контрольная сумма индекса faiss не совпадает")

    encoded = np.frombuffer(index_bytes, dtype=np.uint8)
    return faiss.deserialize_index(encoded)


def _normalize_vectors(vectors: list[list[float]], dimension: int) -> np.ndarray:
    matrix = np.asarray(vectors, dtype=np.float32)
    if matrix.ndim != 2 or matrix.shape[1] != dimension:
        raise ValueError("размерность вектора не совпадает")

    norms = np.linalg.norm(matrix, axis=1, keepdims=True)
    if np.any(norms == 0):
        raise ValueError("нулевой вектор не поддерживается")

    return matrix / norms


def _load_faiss():
    try:
        import faiss
    except ImportError as err:
        raise RuntimeError("faiss-cpu не установлен") from err

    return faiss
