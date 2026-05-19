from dataclasses import dataclass
from datetime import datetime
import hashlib
import uuid
from typing import TYPE_CHECKING

import numpy as np
from psycopg.rows import dict_row

if TYPE_CHECKING:
    from ai_vector_service.db import DatabasePool


@dataclass(frozen=True)
class VectorFrame:
    frame_index: int
    position_millis: int
    vector: list[float]


@dataclass(frozen=True)
class StoredVectorFrame:
    id: int
    ban_id: int
    frame_index: int
    position_millis: int
    vector: list[float]


@dataclass(frozen=True)
class VectorBan:
    id: int
    chat_id: int
    file_unique_id: str
    media_type: str
    model_name: str
    model_revision: str
    vector_dim: int
    frames_count: int
    active: bool
    created_at: str
    frames: list[StoredVectorFrame]


@dataclass(frozen=True)
class IndexState:
    model_name: str
    model_revision: str
    vector_dim: int
    index_type: str
    index_path: str
    active_vectors_count: int
    active_vectors_hash: str
    index_file_sha256: str
    rebuilt_at: str


@dataclass(frozen=True)
class InsertBanResult:
    ban_id: int
    created: bool


class PostgresVectorStore:
    def __init__(self, pool: "DatabasePool") -> None:
        self._pool = pool

    def close(self) -> None:
        return None

    def insert_ban(
        self,
        *,
        chat_id: int,
        file_unique_id: str,
        media_type: str,
        model_name: str,
        model_revision: str,
        vector_dim: int,
        frames: list[VectorFrame],
    ) -> int:
        return self.insert_ban_result(
            chat_id=chat_id,
            file_unique_id=file_unique_id,
            media_type=media_type,
            model_name=model_name,
            model_revision=model_revision,
            vector_dim=vector_dim,
            frames=frames,
        ).ban_id

    def insert_ban_result(
        self,
        *,
        chat_id: int,
        file_unique_id: str,
        media_type: str,
        model_name: str,
        model_revision: str,
        vector_dim: int,
        frames: list[VectorFrame],
    ) -> InsertBanResult:
        self._validate_ban(chat_id, file_unique_id, media_type, model_name, model_revision, vector_dim, frames)
        vector_signature = _vector_signature(frames, vector_dim)

        with self._pool.transaction() as conn:
            conn.row_factory = dict_row
            existing = conn.execute(
                """
SELECT id
FROM ai_vector_ban
WHERE chat_id = %(chat_id)s
  AND model_name = %(model_name)s
  AND model_revision = %(model_revision)s
  AND vector_signature = %(vector_signature)s
""",
                {
                    "chat_id": chat_id,
                    "model_name": model_name,
                    "model_revision": model_revision,
                    "vector_signature": vector_signature,
                },
            ).fetchone()
            if existing is not None:
                return InsertBanResult(ban_id=int(existing["id"]), created=False)

            ban_uid = uuid.uuid4()
            conn.execute(
                """
INSERT INTO media_ban (ban_uid, chat_id, media_type, file_unique_id)
VALUES (%s, %s, %s, %s)
""",
                (ban_uid, chat_id, media_type, file_unique_id),
            )
            ban_row = conn.execute(
                """
INSERT INTO ai_vector_ban (
    ban_uid, chat_id, file_unique_id, media_type, model_name, model_revision,
    vector_dim, frames_count, active, vector_signature
)
VALUES (
    %(ban_uid)s, %(chat_id)s, %(file_unique_id)s, %(media_type)s, %(model_name)s,
    %(model_revision)s, %(vector_dim)s, %(frames_count)s, TRUE, %(vector_signature)s
)
RETURNING id
""",
                {
                    "ban_uid": ban_uid,
                    "chat_id": chat_id,
                    "file_unique_id": file_unique_id,
                    "media_type": media_type,
                    "model_name": model_name,
                    "model_revision": model_revision,
                    "vector_dim": vector_dim,
                    "frames_count": len(frames),
                    "vector_signature": vector_signature,
                },
            ).fetchone()
            if ban_row is None:
                raise RuntimeError("не удалось сохранить ai vector ban")

            ban_id = int(ban_row["id"])
            for frame in frames:
                conn.execute(
                    """
INSERT INTO ai_vector_frame (ban_id, frame_index, position_millis, vector_blob)
VALUES (%s, %s, %s, %s)
ON CONFLICT (ban_id, frame_index) DO NOTHING
""",
                    (ban_id, frame.frame_index, frame.position_millis, _vector_to_blob(frame.vector, vector_dim)),
                )

        return InsertBanResult(ban_id=ban_id, created=True)

    def load_active_bans(
        self,
        *,
        chat_id: int | None = None,
        model_name: str | None = None,
        model_revision: str | None = None,
        vector_dim: int | None = None,
    ) -> list[VectorBan]:
        clauses = ["b.active = TRUE"]
        params: dict[str, object] = {}
        if chat_id is not None:
            clauses.append("b.chat_id = %(chat_id)s")
            params["chat_id"] = chat_id
        if model_name is not None:
            clauses.append("b.model_name = %(model_name)s")
            params["model_name"] = model_name
        if model_revision is not None:
            clauses.append("b.model_revision = %(model_revision)s")
            params["model_revision"] = model_revision
        if vector_dim is not None:
            clauses.append("b.vector_dim = %(vector_dim)s")
            params["vector_dim"] = vector_dim

        with self._pool.raw.connection() as conn:
            conn.row_factory = dict_row
            rows = conn.execute(
                f"""
SELECT b.id AS ban_id, b.chat_id, b.file_unique_id, b.media_type, b.model_name, b.model_revision,
       b.vector_dim, b.frames_count, b.active, b.created_at,
       f.id AS frame_id, f.frame_index, f.position_millis, f.vector_blob
FROM ai_vector_ban b
JOIN ai_vector_frame f ON f.ban_id = b.id
WHERE {" AND ".join(clauses)}
ORDER BY b.id, f.frame_index
""",
                params,
            ).fetchall()

        return _rows_to_bans(rows)

    def deactivate_ban(self, ban_id: int) -> bool:
        with self._pool.transaction() as conn:
            cursor = conn.execute(
                """
WITH deactivated AS (
    UPDATE ai_vector_ban
    SET active = FALSE, updated_at = now(), deactivated_at = COALESCE(deactivated_at, now())
    WHERE id = %s AND active = TRUE
    RETURNING ban_uid
)
UPDATE media_ban
SET active = FALSE, updated_at = now(), deactivated_at = COALESCE(deactivated_at, now())
WHERE ban_uid IN (SELECT ban_uid FROM deactivated)
""",
                (ban_id,),
            )

        return cursor.rowcount > 0

    def active_vectors_count(
        self,
        *,
        chat_id: int | None = None,
        model_name: str,
        model_revision: str,
        vector_dim: int,
    ) -> int:
        clauses = [
            "b.active = TRUE",
            "b.model_name = %(model_name)s",
            "b.model_revision = %(model_revision)s",
            "b.vector_dim = %(vector_dim)s",
        ]
        params: dict[str, object] = {
            "model_name": model_name,
            "model_revision": model_revision,
            "vector_dim": vector_dim,
        }
        if chat_id is not None:
            clauses.append("b.chat_id = %(chat_id)s")
            params["chat_id"] = chat_id

        with self._pool.raw.connection() as conn:
            row = conn.execute(
                """
SELECT COUNT(*) AS count
FROM ai_vector_frame f
JOIN ai_vector_ban b ON b.id = f.ban_id
WHERE """ + " AND ".join(clauses),
                params,
            ).fetchone()

        return int(row["count"] if isinstance(row, dict) else row[0])

    def save_index_state(self, state: IndexState) -> None:
        with self._pool.transaction() as conn:
            conn.execute(
                """
INSERT INTO ai_vector_index_state (
    model_name, model_revision, vector_dim, index_type, index_path,
    active_vectors_count, active_vectors_hash, index_file_sha256, rebuilt_at
)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT(model_name, model_revision, vector_dim, index_type) DO UPDATE SET
    index_path = excluded.index_path,
    active_vectors_count = excluded.active_vectors_count,
    active_vectors_hash = excluded.active_vectors_hash,
    index_file_sha256 = excluded.index_file_sha256,
    rebuilt_at = excluded.rebuilt_at
""",
                (
                    state.model_name,
                    state.model_revision,
                    state.vector_dim,
                    state.index_type,
                    state.index_path,
                    state.active_vectors_count,
                    state.active_vectors_hash,
                    state.index_file_sha256,
                    state.rebuilt_at,
                ),
            )

    def load_index_state(
        self,
        *,
        model_name: str,
        model_revision: str,
        vector_dim: int,
        index_type: str,
    ) -> IndexState | None:
        with self._pool.raw.connection() as conn:
            conn.row_factory = dict_row
            row = conn.execute(
                """
SELECT model_name, model_revision, vector_dim, index_type, index_path,
       active_vectors_count, active_vectors_hash, index_file_sha256, rebuilt_at
FROM ai_vector_index_state
WHERE model_name = %s AND model_revision = %s AND vector_dim = %s AND index_type = %s
""",
                (model_name, model_revision, vector_dim, index_type),
            ).fetchone()

        if row is None:
            return None

        return IndexState(
            model_name=str(row["model_name"]),
            model_revision=str(row["model_revision"]),
            vector_dim=int(row["vector_dim"]),
            index_type=str(row["index_type"]),
            index_path=str(row["index_path"]),
            active_vectors_count=int(row["active_vectors_count"]),
            active_vectors_hash=str(row["active_vectors_hash"]),
            index_file_sha256=str(row["index_file_sha256"]),
            rebuilt_at=_timestamp_to_str(row["rebuilt_at"]),
        )

    def _validate_ban(
        self,
        chat_id: int,
        file_unique_id: str,
        media_type: str,
        model_name: str,
        model_revision: str,
        vector_dim: int,
        frames: list[VectorFrame],
    ) -> None:
        if not isinstance(chat_id, int):
            raise ValueError("chat_id должен быть целым числом")
        if not file_unique_id:
            raise ValueError("file_unique_id пустой")
        if not media_type:
            raise ValueError("media_type пустой")
        if not model_name:
            raise ValueError("model_name пустой")
        if not model_revision:
            raise ValueError("model_revision пустой")
        if vector_dim <= 0:
            raise ValueError("vector_dim должен быть положительным")
        if not frames:
            raise ValueError("требуется хотя бы один вектор кадра")

        seen_frame_indexes = set()
        for frame in frames:
            if frame.frame_index < 0:
                raise ValueError("frame_index не должен быть отрицательным")
            if frame.position_millis < 0:
                raise ValueError("position_millis не должен быть отрицательным")
            if frame.frame_index in seen_frame_indexes:
                raise ValueError("frame_index должен быть уникальным")
            seen_frame_indexes.add(frame.frame_index)
            _vector_to_blob(frame.vector, vector_dim)


def _rows_to_bans(rows: list[dict]) -> list[VectorBan]:
    bans: list[VectorBan] = []
    by_id: dict[int, VectorBan] = {}

    for row in rows:
        ban_id = int(row["ban_id"])
        ban = by_id.get(ban_id)
        if ban is None:
            ban = VectorBan(
                id=ban_id,
                chat_id=int(row["chat_id"]),
                file_unique_id=str(row["file_unique_id"]),
                media_type=str(row["media_type"]),
                model_name=str(row["model_name"]),
                model_revision=str(row["model_revision"]),
                vector_dim=int(row["vector_dim"]),
                frames_count=int(row["frames_count"]),
                active=bool(row["active"]),
                created_at=_timestamp_to_str(row["created_at"]),
                frames=[],
            )
            by_id[ban_id] = ban
            bans.append(ban)

        ban.frames.append(
            StoredVectorFrame(
                id=int(row["frame_id"]),
                ban_id=ban_id,
                frame_index=int(row["frame_index"]),
                position_millis=int(row["position_millis"]),
                vector=_blob_to_vector(row["vector_blob"], ban.vector_dim),
            )
        )

    return bans


def _vector_to_blob(vector: list[float], dimension: int) -> bytes:
    if len(vector) != dimension:
        raise ValueError("размерность вектора не совпадает")

    return np.asarray(vector, dtype=np.float32).tobytes()


def _blob_to_vector(blob: bytes, dimension: int) -> list[float]:
    vector = np.frombuffer(blob, dtype=np.float32)
    if vector.size != dimension:
        raise ValueError("размерность сохраненного вектора не совпадает")

    return vector.astype(np.float32).tolist()


def _vector_signature(frames: list[VectorFrame], dimension: int) -> str:
    digest = hashlib.sha256()
    for frame in frames:
        digest.update(frame.frame_index.to_bytes(4, "big", signed=True))
        digest.update(frame.position_millis.to_bytes(8, "big", signed=True))
        digest.update(_vector_to_blob(frame.vector, dimension))

    return digest.hexdigest()


def _timestamp_to_str(value) -> str:
    if isinstance(value, datetime):
        return value.isoformat()

    return str(value)
