from collections.abc import Iterator
from contextlib import contextmanager
from math import ceil

from psycopg import Connection
from psycopg_pool import ConnectionPool

from ai_vector_service.config import DatabaseSettings


class DatabasePool:
    def __init__(self, pool: ConnectionPool) -> None:
        self._pool = pool

    @property
    def raw(self) -> ConnectionPool:
        return self._pool

    def ping(self) -> None:
        with self._pool.connection() as conn:
            conn.execute("SELECT 1").fetchone()

    def close(self) -> None:
        self._pool.close()

    @contextmanager
    def transaction(self) -> Iterator[Connection]:
        with self._pool.connection() as conn:
            with conn.transaction():
                yield conn


def create_pool(settings: DatabaseSettings) -> DatabasePool:
    pool = ConnectionPool(
        conninfo=settings.dsn,
        min_size=settings.min_size,
        max_size=settings.max_size,
        timeout=settings.connect_timeout,
        kwargs={"connect_timeout": max(1, ceil(settings.connect_timeout))},
        check=ConnectionPool.check_connection,
        open=False,
        name="bastyle-ai-vector",
    )
    pool.open(wait=False)

    return DatabasePool(pool)


def close_pool(pool: DatabasePool | None) -> None:
    if pool is not None:
        pool.close()
