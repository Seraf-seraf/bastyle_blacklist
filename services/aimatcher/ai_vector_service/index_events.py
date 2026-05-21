from __future__ import annotations

import base64
from dataclasses import dataclass
import json
import logging
import socket
import threading
import time
from typing import Protocol

from psycopg.rows import dict_row

try:
    import pika
except ImportError:  # pragma: no cover - dependency is present in production image
    pika = None


LOGGER = logging.getLogger(__name__)
INDEX_NAME = "ai_vector"
SUPPORTED_EVENT_TYPES = {"media.ban.created.v1"}


@dataclass(frozen=True)
class OutboxEvent:
    transaction_id: str
    offset: int
    event_type: str
    aggregate_uid: str
    payload: dict


@dataclass
class IndexCheckpoint:
    consumer_id: str
    index_name: str
    last_transaction_id: str
    last_offset: int
    stale: bool
    stale_reason: str


class OutboxEventReader(Protocol):
    def load_after(self, transaction_id: str, offset: int, limit: int) -> list[OutboxEvent]:
        ...


class CheckpointStore(Protocol):
    def get_or_create(self, consumer_id: str, index_name: str) -> IndexCheckpoint:
        ...

    def update(self, consumer_id: str, index_name: str, transaction_id: str, offset: int) -> None:
        ...

    def mark_stale(self, consumer_id: str, index_name: str, reason: str) -> None:
        ...

    def check_fresh(self, consumer_id: str, index_names: list[str]) -> None:
        ...


class EventApplier(Protocol):
    index_name: str

    def supports(self, event_type: str) -> bool:
        ...

    def apply(self, event: OutboxEvent) -> None:
        ...


class WatermillOutboxEventReader:
    def __init__(self, database_pool) -> None:
        if database_pool is None:
            raise ValueError("PostgreSQL pool не настроен")
        self._pool = database_pool

    def load_after(self, transaction_id: str, offset: int, limit: int) -> list[OutboxEvent]:
        if not transaction_id:
            transaction_id = "0"
        if offset < 0:
            raise ValueError("offset не должен быть отрицательным")
        if limit <= 0:
            raise ValueError("limit должен быть положительным")

        with self._pool.raw.connection() as conn:
            conn.row_factory = dict_row
            rows = conn.execute(
                """
SELECT transaction_id::text, "offset", payload, metadata
FROM watermill_outbox_events
WHERE (transaction_id = %(transaction_id)s::xid8 AND "offset" > %(offset)s)
   OR transaction_id > %(transaction_id)s::xid8
ORDER BY transaction_id ASC, "offset" ASC
LIMIT %(limit)s
""",
                {"transaction_id": transaction_id, "offset": offset, "limit": limit},
            ).fetchall()

        return [_row_to_event(row) for row in rows]


class PostgresCheckpointStore:
    def __init__(self, database_pool) -> None:
        if database_pool is None:
            raise ValueError("PostgreSQL pool не настроен")
        self._pool = database_pool

    def get_or_create(self, consumer_id: str, index_name: str) -> IndexCheckpoint:
        _validate_checkpoint_key(consumer_id, index_name)
        with self._pool.transaction() as conn:
            conn.row_factory = dict_row
            conn.execute(
                """
INSERT INTO index_checkpoints (consumer_id, index_name)
VALUES (%(consumer_id)s, %(index_name)s)
ON CONFLICT (consumer_id, index_name) DO NOTHING
""",
                {"consumer_id": consumer_id, "index_name": index_name},
            )
            row = conn.execute(
                """
SELECT consumer_id, index_name, last_applied_transaction_id::text, last_applied_offset,
       stale, COALESCE(stale_reason, '') AS stale_reason
FROM index_checkpoints
WHERE consumer_id = %(consumer_id)s AND index_name = %(index_name)s
""",
                {"consumer_id": consumer_id, "index_name": index_name},
            ).fetchone()

        return IndexCheckpoint(
            consumer_id=str(row["consumer_id"]),
            index_name=str(row["index_name"]),
            last_transaction_id=str(row["last_applied_transaction_id"]),
            last_offset=int(row["last_applied_offset"]),
            stale=bool(row["stale"]),
            stale_reason=str(row["stale_reason"]),
        )

    def update(self, consumer_id: str, index_name: str, transaction_id: str, offset: int) -> None:
        _validate_checkpoint_key(consumer_id, index_name)
        if not transaction_id:
            raise ValueError("transaction_id обязателен")
        if offset < 0:
            raise ValueError("offset не должен быть отрицательным")

        with self._pool.transaction() as conn:
            conn.execute(
                """
UPDATE index_checkpoints
SET last_applied_transaction_id = %(transaction_id)s::xid8,
    last_applied_offset = %(offset)s,
    updated_at = now(),
    stale = FALSE,
    stale_reason = NULL
WHERE consumer_id = %(consumer_id)s
  AND index_name = %(index_name)s
  AND (
        last_applied_transaction_id < %(transaction_id)s::xid8
        OR (last_applied_transaction_id = %(transaction_id)s::xid8 AND last_applied_offset <= %(offset)s)
      )
""",
                {
                    "consumer_id": consumer_id,
                    "index_name": index_name,
                    "transaction_id": transaction_id,
                    "offset": offset,
                },
            )

    def mark_stale(self, consumer_id: str, index_name: str, reason: str) -> None:
        _validate_checkpoint_key(consumer_id, index_name)
        if not reason:
            raise ValueError("причина stale обязательна")
        with self._pool.transaction() as conn:
            conn.execute(
                """
UPDATE index_checkpoints
SET stale = TRUE,
    stale_reason = %(reason)s,
    updated_at = now()
WHERE consumer_id = %(consumer_id)s
  AND index_name = %(index_name)s
""",
                {"consumer_id": consumer_id, "index_name": index_name, "reason": reason},
            )

    def check_fresh(self, consumer_id: str, index_names: list[str]) -> None:
        if not consumer_id:
            raise ValueError("consumer_id обязателен")
        if not index_names:
            return
        with self._pool.raw.connection() as conn:
            stale_count = conn.execute(
                """
SELECT count(*)
FROM index_checkpoints
WHERE consumer_id = %s
  AND index_name = ANY(%s)
  AND stale = TRUE
""",
                (consumer_id, index_names),
            ).fetchone()[0]
        if stale_count:
            raise RuntimeError("найдены stale index checkpoints")


class IndexHealthCheck:
    def __init__(self, checkpoints: CheckpointStore, consumer_id: str, index_names: list[str]) -> None:
        if checkpoints is None:
            raise ValueError("checkpoint store не настроен")
        if not consumer_id:
            raise ValueError("consumer_id обязателен")
        self._checkpoints = checkpoints
        self._consumer_id = consumer_id
        self._index_names = index_names

    def check(self) -> None:
        self._checkpoints.check_fresh(self._consumer_id, self._index_names)


class AIVectorEventApplier:
    index_name = INDEX_NAME

    def __init__(self, vector_index_service) -> None:
        if vector_index_service is None:
            raise ValueError("AI-vector index service не настроен")
        self._service = vector_index_service

    def supports(self, event_type: str) -> bool:
        return event_type in SUPPORTED_EVENT_TYPES

    def apply(self, event: OutboxEvent) -> None:
        self._service.apply_existing_ban(ban_uid=event.aggregate_uid)


class IndexSynchronizer:
    def __init__(
        self,
        *,
        consumer_id: str,
        reader: OutboxEventReader,
        checkpoints: CheckpointStore,
        applier: EventApplier,
        batch_size: int,
    ) -> None:
        if not consumer_id:
            raise ValueError("consumer_id обязателен")
        if reader is None:
            raise ValueError("reader outbox-событий не настроен")
        if checkpoints is None:
            raise ValueError("checkpoint store не настроен")
        if applier is None:
            raise ValueError("index applier не настроен")
        if batch_size <= 0:
            raise ValueError("batch_size должен быть положительным")

        self._consumer_id = consumer_id
        self._reader = reader
        self._checkpoints = checkpoints
        self._applier = applier
        self._batch_size = batch_size

    def catch_up(self) -> None:
        checkpoint = self._checkpoints.get_or_create(self._consumer_id, self._applier.index_name)
        last_transaction_id = checkpoint.last_transaction_id
        last_offset = checkpoint.last_offset

        while True:
            events = self._reader.load_after(last_transaction_id, last_offset, self._batch_size)
            if not events:
                return

            batch_transaction_id = last_transaction_id
            batch_offset = last_offset
            for event in events:
                if not _position_after(event.transaction_id, event.offset, batch_transaction_id, batch_offset):
                    continue
                if self._applier.supports(event.event_type):
                    try:
                        self._applier.apply(event)
                    except Exception as err:
                        reason = f"failed to apply event {event.aggregate_uid} to {self._applier.index_name}: {err}"
                        self._checkpoints.mark_stale(self._consumer_id, self._applier.index_name, reason)
                        raise
                batch_transaction_id = event.transaction_id
                batch_offset = event.offset

            if _position_after(batch_transaction_id, batch_offset, last_transaction_id, last_offset):
                self._checkpoints.update(
                    self._consumer_id,
                    self._applier.index_name,
                    batch_transaction_id,
                    batch_offset,
                )
                last_transaction_id = batch_transaction_id
                last_offset = batch_offset

            if len(events) < self._batch_size:
                return


class RabbitMQIndexConsumer:
    def __init__(
        self,
        *,
        rabbitmq_url: str,
        exchange: str,
        exchange_type: str,
        queue_template: str,
        routing_keys: list[str],
        prefetch: int,
        synchronizer: IndexSynchronizer,
        consumer_id: str | None = None,
        reconnect_interval: float = 5.0,
        catch_up_interval: float = 5.0,
    ) -> None:
        if pika is None:
            raise RuntimeError("pika не установлен")
        if not rabbitmq_url:
            raise ValueError("RabbitMQ URL обязателен")
        if not exchange:
            raise ValueError("RabbitMQ exchange обязателен")
        if not exchange_type:
            raise ValueError("RabbitMQ exchange type обязателен")
        if not queue_template:
            raise ValueError("queue_template обязателен")
        if not routing_keys:
            raise ValueError("routing_keys обязательны")
        if prefetch <= 0:
            raise ValueError("prefetch должен быть положительным")
        if reconnect_interval <= 0:
            raise ValueError("reconnect_interval должен быть положительным")
        if catch_up_interval <= 0:
            raise ValueError("catch_up_interval должен быть положительным")
        if synchronizer is None:
            raise ValueError("synchronizer не настроен")

        self._rabbitmq_url = rabbitmq_url
        self._exchange = exchange
        self._exchange_type = exchange_type
        self._queue_name = queue_template % (consumer_id or socket.gethostname())
        self._routing_keys = routing_keys
        self._prefetch = prefetch
        self._synchronizer = synchronizer
        self._reconnect_interval = reconnect_interval
        self._catch_up_interval = catch_up_interval
        self._stop = threading.Event()

    def start_background(self) -> threading.Thread:
        LOGGER.info("Начинается bootstrap catch-up AI-vector индекса")
        self._synchronizer.catch_up()
        LOGGER.info("Bootstrap catch-up AI-vector индекса завершен")
        thread = threading.Thread(target=self.run, name="ai-vector-index-consumer", daemon=True)
        thread.start()
        return thread

    def stop(self) -> None:
        self._stop.set()

    def run(self) -> None:
        while not self._stop.is_set():
            try:
                self._run_session()
            except Exception as err:
                LOGGER.error("Ошибка RabbitMQ consumer AI-vector индекса: %s", err)
                self._stop.wait(self._reconnect_interval)

    def _run_session(self) -> None:
        parameters = pika.URLParameters(self._rabbitmq_url)
        connection = pika.BlockingConnection(parameters)
        try:
            channel = connection.channel()
            channel.exchange_declare(exchange=self._exchange, exchange_type=self._exchange_type, durable=True)
            channel.queue_declare(
                queue=self._queue_name,
                durable=True,
                exclusive=False,
                auto_delete=False,
                arguments={"x-queue-type": "quorum"},
            )
            for routing_key in self._routing_keys:
                channel.queue_bind(queue=self._queue_name, exchange=self._exchange, routing_key=routing_key)
            channel.basic_qos(prefetch_count=self._prefetch)

            last_catch_up = 0.0
            while not self._stop.is_set():
                method, _, _ = channel.basic_get(queue=self._queue_name, auto_ack=False)
                if method is not None:
                    try:
                        self._synchronizer.catch_up()
                    except Exception:
                        channel.basic_nack(method.delivery_tag, requeue=True)
                    else:
                        channel.basic_ack(method.delivery_tag)
                    continue

                now = time.monotonic()
                if now - last_catch_up >= self._catch_up_interval:
                    self._synchronizer.catch_up()
                    last_catch_up = now
                self._stop.wait(min(0.5, self._catch_up_interval))
        finally:
            connection.close()


def _row_to_event(row) -> OutboxEvent:
    metadata = _json_value(row["metadata"] or {})
    payload = _json_value(row["payload"] or b"{}")
    destination_topic = ""
    if isinstance(payload, dict) and payload.get("destination_topic"):
        envelope = payload
        destination_topic = str(envelope.get("destination_topic", ""))
        metadata = envelope.get("metadata") or metadata
        payload = _decode_envelope_payload(envelope.get("payload", "e30="))

    return OutboxEvent(
        transaction_id=str(row["transaction_id"]),
        offset=int(row["offset"]),
        event_type=str(metadata.get("event_type") or destination_topic),
        aggregate_uid=str(metadata.get("aggregate_uid", "")),
        payload=payload,
    )


def _json_value(value):
    if isinstance(value, memoryview):
        value = value.tobytes()
    if isinstance(value, bytes):
        value = value.decode("utf-8")
    if isinstance(value, str):
        return json.loads(value)
    return value


def _decode_envelope_payload(value) -> dict:
    if isinstance(value, str):
        raw = base64.b64decode(value)
        return json.loads(raw.decode("utf-8"))
    return _json_value(value)


def _validate_checkpoint_key(consumer_id: str, index_name: str) -> None:
    if not consumer_id:
        raise ValueError("consumer_id обязателен")
    if not index_name:
        raise ValueError("index_name обязателен")


def _position_after(transaction_id: str, offset: int, last_transaction_id: str, last_offset: int) -> bool:
    current = int(transaction_id)
    last = int(last_transaction_id)
    if current > last:
        return True
    return current == last and offset > last_offset
