from contextlib import contextmanager

import pytest

from ai_vector_service.config import DatabaseSettings
from ai_vector_service import db


class FakeConnection:
    def __init__(self):
        self.executed = []

    def execute(self, query):
        self.executed.append(query)
        return self

    def fetchone(self):
        return (1,)

    @contextmanager
    def transaction(self):
        yield self


class FakeConnectionContext:
    def __init__(self, connection):
        self.connection = connection

    def __enter__(self):
        return self.connection

    def __exit__(self, exc_type, exc, tb):
        return False


class FakeConnectionPool:
    check_connection = object()

    def __init__(self, **kwargs):
        self.kwargs = kwargs
        self.connection_instance = FakeConnection()
        self.open_calls = []
        self.closed = False

    def open(self, *, wait):
        self.open_calls.append(wait)

    def close(self):
        self.closed = True

    def connection(self):
        return FakeConnectionContext(self.connection_instance)


def test_create_pool_configures_psycopg_pool(monkeypatch):
    created_pools = []

    def fake_pool_factory(**kwargs):
        pool = FakeConnectionPool(**kwargs)
        created_pools.append(pool)
        return pool

    monkeypatch.setattr(db, "ConnectionPool", fake_pool_factory)
    fake_pool_factory.check_connection = FakeConnectionPool.check_connection

    pool = db.create_pool(
        DatabaseSettings(
            dsn="postgres://user:pass@db:5432/app?sslmode=disable",
            min_size=2,
            max_size=12,
            connect_timeout=6,
        )
    )

    assert pool.raw is created_pools[0]
    assert created_pools[0].kwargs["conninfo"] == "postgres://user:pass@db:5432/app?sslmode=disable"
    assert created_pools[0].kwargs["min_size"] == 2
    assert created_pools[0].kwargs["max_size"] == 12
    assert created_pools[0].kwargs["timeout"] == 6
    assert created_pools[0].kwargs["kwargs"] == {"connect_timeout": 6}
    assert created_pools[0].kwargs["open"] is False
    assert created_pools[0].open_calls == [False]


def test_database_pool_ping_and_close():
    raw_pool = FakeConnectionPool()
    pool = db.DatabasePool(raw_pool)

    pool.ping()
    pool.close()

    assert raw_pool.connection_instance.executed == ["SELECT 1"]
    assert raw_pool.closed


def test_close_pool_ignores_missing_pool():
    db.close_pool(None)


def test_close_pool_closes_database_pool():
    raw_pool = FakeConnectionPool()
    pool = db.DatabasePool(raw_pool)

    db.close_pool(pool)

    assert raw_pool.closed


def test_database_pool_transaction_rolls_back_on_error():
    raw_pool = FakeConnectionPool()
    pool = db.DatabasePool(raw_pool)

    with pytest.raises(RuntimeError):
        with pool.transaction() as conn:
            conn.execute("INSERT")
            raise RuntimeError("ошибка транзакции")

    assert raw_pool.connection_instance.executed == ["INSERT"]
