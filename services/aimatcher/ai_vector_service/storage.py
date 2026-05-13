from dataclasses import dataclass
from importlib import resources
from pathlib import Path
import sqlite3

import numpy as np

MIGRATIONS_PACKAGE = "ai_vector_service.migrations"
MIGRATIONS_TABLE = "ai_vector_schema_migrations"
UP_MARKER = "-- +bastyle Up"
DOWN_MARKER = "-- +bastyle Down"


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


class SQLiteVectorStore:
    def __init__(self, path: str | Path) -> None:
        if not str(path):
            raise ValueError("путь к SQLite пустой")

        self._db = sqlite3.connect(path)
        self._db.row_factory = sqlite3.Row
        self._ensure_schema()

    def close(self) -> None:
        self._db.close()

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
        self._validate_ban(chat_id, file_unique_id, media_type, model_name, model_revision, vector_dim, frames)

        with self._db:
            cursor = self._db.execute(
                """
INSERT INTO ai_vector_ban (
    chat_id, file_unique_id, media_type, model_name, model_revision, vector_dim, frames_count, active
)
VALUES (?, ?, ?, ?, ?, ?, ?, 1)
""",
                (chat_id, file_unique_id, media_type, model_name, model_revision, vector_dim, len(frames)),
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
        chat_id: int | None = None,
        model_name: str | None = None,
        model_revision: str | None = None,
        vector_dim: int | None = None,
    ) -> list[VectorBan]:
        clauses = ["b.active = 1"]
        params: list[object] = []
        if chat_id is not None:
            clauses.append("b.chat_id = ?")
            params.append(chat_id)
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
        chat_id: int | None = None,
        model_name: str,
        model_revision: str,
        vector_dim: int,
    ) -> int:
        clauses = [
            "b.active = 1",
            "b.model_name = ?",
            "b.model_revision = ?",
            "b.vector_dim = ?",
        ]
        params: list[object] = [model_name, model_revision, vector_dim]
        if chat_id is not None:
            clauses.append("b.chat_id = ?")
            params.append(chat_id)

        row = self._db.execute(
            """
SELECT COUNT(*) AS count
FROM ai_vector_frame f
JOIN ai_vector_ban b ON b.id = f.ban_id
WHERE """ + " AND ".join(clauses),
            params,
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
            f"""
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS {MIGRATIONS_TABLE} (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
"""
        )
        _apply_migrations(self._db)

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


def _rows_to_bans(rows: list[sqlite3.Row]) -> list[VectorBan]:
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
        raise ValueError("размерность вектора не совпадает")

    return np.asarray(vector, dtype=np.float32).tobytes()


def _blob_to_vector(blob: bytes, dimension: int) -> list[float]:
    vector = np.frombuffer(blob, dtype=np.float32)
    if vector.size != dimension:
        raise ValueError("размерность сохраненного вектора не совпадает")

    return vector.astype(np.float32).tolist()


def _apply_migrations(db: sqlite3.Connection) -> None:
    applied_versions = _applied_migration_versions(db)
    for migration in _load_migrations():
        if migration.version in applied_versions:
            continue

        with db:
            db.executescript(migration.up_sql)
            db.execute(
                f"INSERT INTO {MIGRATIONS_TABLE} (version, name) VALUES (?, ?)",
                (migration.version, migration.name),
            )


@dataclass(frozen=True)
class _Migration:
    version: int
    name: str
    up_sql: str


def _applied_migration_versions(db: sqlite3.Connection) -> set[int]:
    rows = db.execute(f"SELECT version FROM {MIGRATIONS_TABLE}").fetchall()
    return {int(row["version"]) for row in rows}


def _load_migrations() -> list[_Migration]:
    migrations: list[_Migration] = []
    for migration_file in sorted(resources.files(MIGRATIONS_PACKAGE).iterdir()):
        if migration_file.suffix != ".sql":
            continue

        version, name = _parse_migration_name(migration_file.name)
        sql = migration_file.read_text(encoding="utf-8")
        migrations.append(_Migration(version=version, name=name, up_sql=_extract_up_sql(sql)))

    return migrations


def _parse_migration_name(filename: str) -> tuple[int, str]:
    version_raw, separator, name = filename.partition("_")
    if separator == "" or not version_raw.isdigit():
        raise ValueError(f"некорректное имя миграции: {filename}")

    return int(version_raw), name.removesuffix(".sql")


def _extract_up_sql(sql: str) -> str:
    up_index = sql.find(UP_MARKER)
    if up_index == -1:
        raise ValueError(f"миграция должна содержать {UP_MARKER}")

    down_index = sql.find(DOWN_MARKER, up_index + len(UP_MARKER))
    if down_index == -1:
        return sql[up_index + len(UP_MARKER) :].strip()

    return sql[up_index + len(UP_MARKER) : down_index].strip()
