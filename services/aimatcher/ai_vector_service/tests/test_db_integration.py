from testcontainers.postgres import PostgresContainer

from ai_vector_service.config import DatabaseSettings
from ai_vector_service.db import create_pool


def test_database_pool_with_postgres_container():
    with PostgresContainer("postgres:16-alpine", username="bastyle", password="bastyle", dbname="bastyle") as postgres:
        pool = create_pool(
            DatabaseSettings(
                dsn=postgres.get_connection_url(driver=None),
                min_size=1,
                max_size=2,
                connect_timeout=5,
            )
        )
        try:
            pool.ping()
            with pool.transaction() as conn:
                conn.execute("CREATE TABLE tx_check(id BIGINT PRIMARY KEY)")
                conn.execute("INSERT INTO tx_check(id) VALUES (1)")

            with pool.raw.connection() as conn:
                count = conn.execute("SELECT count(*) FROM tx_check").fetchone()[0]
            assert count == 1
        finally:
            pool.close()
