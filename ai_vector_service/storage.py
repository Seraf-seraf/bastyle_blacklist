from dataclasses import dataclass
from pathlib import Path
import sqlite3

import numpy as np


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


class SQLiteVectorStore:
    def __init__(self, path: str | Path) -> None:
        if not str(path):
            raise ValueError("sqlite path is empty")

        self._db = sqlite3.connect(path)
        self._db.row_factory = sqlite3.Row
        self._ensure_schema()

    def close(self) -> None:
        self._db.close()

    def insert_ban(
        self,
        *,
        file_unique_id: str,
        media_type: str,
        model_name: str,
        model_revision: str,
        vector_dim: int,
        frames: list[VectorFrame],
    ) -> int:
        self._validate_ban(file_unique_id, media_type, model_name, model_revision, vector_dim, frames)

        with self._db:
            cursor = self._db.execute(
                """
INSERT INTO ai_vector_ban (
    file_unique_id, media_type, model_name, model_revision, vector_dim, frames_count, active
)
VALUES (?, ?, ?, ?, ?, ?, 1)
""",
                (file_unique_id, media_type, model_name, model_revision, vector_dim, len(frames)),
            )
            ban_id = int(cursor.lastrowid)

            for frame in frames:
                self._db.execute(
                    """
INSERT INTO ai_vector_frame (
    ban_id, frame_index, position_millis, vector_blob
)
VALUES (?, ?, ?, ?)
""",
                    (
                        ban_id,
                        frame.frame_index,
                        frame.position_millis,
                        _vector_to_blob(frame.vector, vector_dim),
                    ),
                )

        return ban_id

    def load_active_bans(
        self,
        *,
        model_name: str | None = None,
        model_revision: str | None = None,
        vector_dim: int | None = None,
    ) -> list[VectorBan]:
        clauses = ["b.active = 1"]
        params: list[object] = []
        if model_name is not None:
            clauses.append("b.model_name = ?")
            params.append(model_name)
        if model_revision is not None:
            clauses.append("b.model_revision = ?")
            params.append(model_revision)
        if vector_dim is not None:
            clauses.append("b.vector_dim = ?")
            params.append(vector_dim)

        rows = self._db.execute(
            f"""
SELECT b.id AS ban_id, b.file_unique_id, b.media_type, b.model_name, b.model_revision,
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
        with self._db:
            cursor = self._db.execute(
                """
UPDATE ai_vector_ban
SET active = 0
WHERE id = ? AND active = 1
""",
                (ban_id,),
            )

        return cursor.rowcount > 0

    def active_vectors_count(
        self,
        *,
        model_name: str,
        model_revision: str,
        vector_dim: int,
    ) -> int:
        row = self._db.execute(
            """
SELECT COUNT(*) AS count
FROM ai_vector_frame f
JOIN ai_vector_ban b ON b.id = f.ban_id
WHERE b.active = 1
  AND b.model_name = ?
  AND b.model_revision = ?
  AND b.vector_dim = ?
""",
            (model_name, model_revision, vector_dim),
        ).fetchone()

        return int(row["count"])

    def save_index_state(self, state: IndexState) -> None:
        with self._db:
            self._db.execute(
                """
INSERT INTO ai_vector_index_state (
    model_name, model_revision, vector_dim, index_type, index_path,
    active_vectors_count, active_vectors_hash, index_file_sha256, rebuilt_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
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
        row = self._db.execute(
            """
SELECT model_name, model_revision, vector_dim, index_type, index_path,
       active_vectors_count, active_vectors_hash, index_file_sha256, rebuilt_at
FROM ai_vector_index_state
WHERE model_name = ? AND model_revision = ? AND vector_dim = ? AND index_type = ?
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
            rebuilt_at=str(row["rebuilt_at"]),
        )

    def _ensure_schema(self) -> None:
        self._db.executescript(
            """
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS ai_vector_ban (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_unique_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    model_name TEXT NOT NULL,
    model_revision TEXT NOT NULL,
    vector_dim INTEGER NOT NULL,
    frames_count INTEGER NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ai_vector_frame (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ban_id INTEGER NOT NULL,
    frame_index INTEGER NOT NULL,
    position_millis INTEGER NOT NULL,
    vector_blob BLOB NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (ban_id, frame_index),
    FOREIGN KEY (ban_id) REFERENCES ai_vector_ban(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS ai_vector_index_state (
    model_name TEXT NOT NULL,
    model_revision TEXT NOT NULL,
    vector_dim INTEGER NOT NULL,
    index_type TEXT NOT NULL,
    index_path TEXT NOT NULL,
    active_vectors_count INTEGER NOT NULL,
    active_vectors_hash TEXT NOT NULL DEFAULT '',
    index_file_sha256 TEXT NOT NULL DEFAULT '',
    rebuilt_at TEXT NOT NULL,
    PRIMARY KEY (model_name, model_revision, vector_dim, index_type)
);

CREATE INDEX IF NOT EXISTS ai_vector_ban_active_model_idx
ON ai_vector_ban(active, model_name, model_revision, vector_dim);

CREATE INDEX IF NOT EXISTS ai_vector_frame_ban_idx
ON ai_vector_frame(ban_id, frame_index);
"""
        )
        self._ensure_index_state_columns()

    def _ensure_index_state_columns(self) -> None:
        rows = self._db.execute("PRAGMA table_info(ai_vector_index_state)").fetchall()
        columns = {str(row["name"]) for row in rows}

        if "active_vectors_hash" not in columns:
            self._db.execute(
                "ALTER TABLE ai_vector_index_state "
                "ADD COLUMN active_vectors_hash TEXT NOT NULL DEFAULT ''"
            )
        if "index_file_sha256" not in columns:
            self._db.execute(
                "ALTER TABLE ai_vector_index_state "
                "ADD COLUMN index_file_sha256 TEXT NOT NULL DEFAULT ''"
            )

    def _validate_ban(
        self,
        file_unique_id: str,
        media_type: str,
        model_name: str,
        model_revision: str,
        vector_dim: int,
        frames: list[VectorFrame],
    ) -> None:
        if not file_unique_id:
            raise ValueError("file_unique_id is empty")
        if not media_type:
            raise ValueError("media_type is empty")
        if not model_name:
            raise ValueError("model_name is empty")
        if not model_revision:
            raise ValueError("model_revision is empty")
        if vector_dim <= 0:
            raise ValueError("vector_dim must be positive")
        if not frames:
            raise ValueError("at least one vector frame is required")

        seen_frame_indexes = set()
        for frame in frames:
            if frame.frame_index < 0:
                raise ValueError("frame_index must be non-negative")
            if frame.position_millis < 0:
                raise ValueError("position_millis must be non-negative")
            if frame.frame_index in seen_frame_indexes:
                raise ValueError("frame_index must be unique")
            seen_frame_indexes.add(frame.frame_index)
            _vector_to_blob(frame.vector, vector_dim)


def _rows_to_bans(rows: list[sqlite3.Row]) -> list[VectorBan]:
    bans: list[VectorBan] = []
    by_id: dict[int, VectorBan] = {}

    for row in rows:
        ban_id = int(row["ban_id"])
        ban = by_id.get(ban_id)
        if ban is None:
            ban = VectorBan(
                id=ban_id,
                file_unique_id=str(row["file_unique_id"]),
                media_type=str(row["media_type"]),
                model_name=str(row["model_name"]),
                model_revision=str(row["model_revision"]),
                vector_dim=int(row["vector_dim"]),
                frames_count=int(row["frames_count"]),
                active=bool(row["active"]),
                created_at=str(row["created_at"]),
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
        raise ValueError("vector dimension mismatch")

    return np.asarray(vector, dtype=np.float32).tobytes()


def _blob_to_vector(blob: bytes, dimension: int) -> list[float]:
    vector = np.frombuffer(blob, dtype=np.float32)
    if vector.size != dimension:
        raise ValueError("stored vector dimension mismatch")

    return vector.astype(np.float32).tolist()
